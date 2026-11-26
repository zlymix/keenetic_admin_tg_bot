package bot

import (
	"context"
	"fmt"
	"html"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"keenetic_admin/internal/keenetic"
	"keenetic_admin/internal/storage"

	tele "gopkg.in/telebot.v4"
)

func escapeHTML(s string) string {
	return html.EscapeString(s)
}

func sendOptions(markup *tele.ReplyMarkup) *tele.SendOptions {
	return &tele.SendOptions{
		ParseMode:   tele.ModeHTML,
		ReplyMarkup: markup,
	}
}

// Inline button endpoints for native telebot routing
var (
	btnDevShow      = tele.Btn{Unique: "dev_show"}
	btnDevAccess    = tele.Btn{Unique: "dev_access"}
	btnDevFav       = tele.Btn{Unique: "dev_fav"}
	btnDevWOL       = tele.Btn{Unique: "dev_wol"}
	btnDevSpeedMenu = tele.Btn{Unique: "dev_speed_menu"}
	btnDevSpeedSet  = tele.Btn{Unique: "dev_speed_set"}
	btnDevList      = tele.Btn{Unique: "dev_list"}
	btnInetRefresh  = tele.Btn{Unique: "inet_refresh"}
	btnRebootPrompt = tele.Btn{Unique: "reboot_prompt"}
	btnRebootExec   = tele.Btn{Unique: "reboot_confirm"}
	btnRebootCancel = tele.Btn{Unique: "reboot_cancel"}
	btnSysRefresh     = tele.Btn{Unique: "sys_refresh"}
	btnDevText        = tele.Btn{Unique: "dev_text"}
	btnDevTextRefresh = tele.Btn{Unique: "dev_text_refresh"}
)

// Bot handles Telegram bot interactions.
type Bot struct {
	teleBot   *tele.Bot
	client    *keenetic.Client
	monitor   *DeviceMonitor
	favorites *storage.FavoritesStorage
	history   *storage.HistoryStorage
}

func New(teleBot *tele.Bot, client *keenetic.Client, monitor *DeviceMonitor, favorites *storage.FavoritesStorage, history *storage.HistoryStorage) *Bot {
	return &Bot{
		teleBot:   teleBot,
		client:    client,
		monitor:   monitor,
		favorites: favorites,
		history:   history,
	}
}

// SetupRoutes registers all command and callback handlers.
func (b *Bot) SetupRoutes() {
	// Main reply keyboard
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	btnDevices := menu.Text("📱 Устройства")
	btnActivity := menu.Text("⏱ По времени")
	btnInternet := menu.Text("🌐 Интернет")
	btnSystem := menu.Text("ℹ️ Роутер")
	btnReboot := menu.Text("🔄 Перезагрузка")

	menu.Reply(
		menu.Row(btnDevices, btnActivity),
		menu.Row(btnInternet, btnSystem),
		menu.Row(btnReboot),
	)

	// Command /start & /menu
	b.teleBot.Handle("/start", func(c tele.Context) error {
		welcome := "👋 <b>Панель управления Keenetic</b>\n\n" +
			"Используйте кнопки меню ниже для управления роутером:\n" +
			"• <b>📱 Устройства</b> — список подключенных устройств, избранное, отключение интернета, ограничение скорости, Wake-on-LAN\n" +
			"• <b>⏱ По времени</b> (или <b>/online</b>) — список устройств текстом по времени онлайн / офлайн\n" +
			"• <b>🌐 Интернет</b> — состояние провайдера и VPN-подключений\n" +
			"• <b>ℹ️ Роутер</b> — системная информация, CPU, память, аптайм\n" +
			"• <b>🔄 Перезагрузка</b> — перезапуск роутера"
		return c.Send(welcome, sendOptions(menu))
	})

	b.teleBot.Handle("/menu", func(c tele.Context) error {
		return c.Send("Главное меню:", sendOptions(menu))
	})

	// Text commands for device activity list
	b.teleBot.Handle("/devices", func(c tele.Context) error {
		return b.handleDevicesActivityText(c, false)
	})
	b.teleBot.Handle("/online", func(c tele.Context) error {
		return b.handleDevicesActivityText(c, false)
	})
	b.teleBot.Handle("/activity", func(c tele.Context) error {
		return b.handleDevicesActivityText(c, false)
	})
	b.teleBot.Handle("/uptime", func(c tele.Context) error {
		return b.handleDevicesActivityText(c, false)
	})

	// Reply Keyboard Buttons
	b.teleBot.Handle(&btnDevices, func(c tele.Context) error {
		return b.handleDevicesList(c, true, false)
	})

	b.teleBot.Handle(&btnActivity, func(c tele.Context) error {
		return b.handleDevicesActivityText(c, false)
	})

	b.teleBot.Handle(&btnInternet, func(c tele.Context) error {
		return b.handleInternetStatus(c, false)
	})

	b.teleBot.Handle(&btnReboot, func(c tele.Context) error {
		return b.handleRebootPrompt(c, false)
	})

	b.teleBot.Handle(&btnSystem, func(c tele.Context) error {
		return b.handleSystemInfo(c, false)
	})

	// Native Button Callback Handlers

	// 1. Show Device Card
	b.teleBot.Handle(&btnDevShow, func(c tele.Context) error {
		mac := c.Data()
		return b.handleDeviceCard(c, mac, true)
	})

	// 2. Toggle Device Access (Permit / Deny)
	b.teleBot.Handle(&btnDevAccess, func(c tele.Context) error {
		parts := strings.Split(c.Data(), "|")
		if len(parts) >= 2 {
			return b.handleDeviceAccessToggle(c, parts[0], parts[1] == "permit")
		}
		return c.Respond()
	})

	// 3. Toggle Favorite Status
	b.teleBot.Handle(&btnDevFav, func(c tele.Context) error {
		mac := c.Data()
		if mac != "" {
			return b.handleFavoriteToggle(c, mac)
		}
		return c.Respond()
	})

	// 4. Wake on LAN
	b.teleBot.Handle(&btnDevWOL, func(c tele.Context) error {
		mac := c.Data()
		return b.handleDeviceWOL(c, mac)
	})

	// 5. Speed Menu
	b.teleBot.Handle(&btnDevSpeedMenu, func(c tele.Context) error {
		mac := c.Data()
		return b.handleSpeedMenu(c, mac)
	})

	// 6. Apply Speed Limit
	b.teleBot.Handle(&btnDevSpeedSet, func(c tele.Context) error {
		parts := strings.Split(c.Data(), "|")
		if len(parts) >= 2 {
			rate, _ := strconv.Atoi(parts[1])
			return b.handleSpeedSet(c, parts[0], rate)
		}
		return c.Respond()
	})

	// 7. Device List Navigation
	b.teleBot.Handle(&btnDevList, func(c tele.Context) error {
		onlineOnly := c.Data() != "all"
		return b.handleDevicesList(c, onlineOnly, true)
	})

	// 8. Internet Status Refresh
	b.teleBot.Handle(&btnInetRefresh, func(c tele.Context) error {
		return b.handleInternetStatus(c, true)
	})

	// 9. Reboot Handlers
	b.teleBot.Handle(&btnRebootPrompt, func(c tele.Context) error {
		return b.handleRebootPrompt(c, true)
	})
	b.teleBot.Handle(&btnRebootExec, func(c tele.Context) error {
		return b.handleRebootExecute(c)
	})
	b.teleBot.Handle(&btnRebootCancel, func(c tele.Context) error {
		return b.handleRebootCancel(c)
	})

	// 10. System Info Refresh
	b.teleBot.Handle(&btnSysRefresh, func(c tele.Context) error {
		return b.handleSystemInfo(c, true)
	})

	// 11. Device Activity Text
	b.teleBot.Handle(&btnDevText, func(c tele.Context) error {
		return b.handleDevicesActivityText(c, true)
	})
	b.teleBot.Handle(&btnDevTextRefresh, func(c tele.Context) error {
		return b.handleDevicesActivityText(c, true)
	})
}

// editOrIgnore edits a message, responding to the callback and ignoring "message is not modified" errors.
func (b *Bot) editOrIgnore(c tele.Context, text string, markup *tele.ReplyMarkup, toastText string) error {
	if toastText != "" {
		_ = c.Respond(&tele.CallbackResponse{Text: toastText})
	} else {
		_ = c.Respond()
	}

	err := c.Edit(text, sendOptions(markup))
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "not modified") {
		return nil
	}
	return err
}

// handleDevicesList renders the list of devices (active only or all).
// Favorite devices are sorted to the very top.
func (b *Bot) handleDevicesList(c tele.Context, onlineOnly bool, isEdit bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hotspot, err := b.client.GetHotspot(ctx)
	if err != nil {
		log.Printf("[BOT] Error getting hotspot: %v", err)
		msg := fmt.Sprintf("❌ Ошибка при получении списка устройств: %v", err)
		if isEdit {
			return b.editOrIgnore(c, msg, nil, "Ошибка получения данных")
		}
		return c.Send(msg)
	}

	var displayedHosts []keenetic.Host
	var activeCount, blockedCount, favCount int

	for _, h := range hotspot.Host {
		if h.Active {
			activeCount++
		}
		if h.IsBlocked() {
			blockedCount++
		}
		if b.favorites != nil && b.favorites.IsFavorite(h.MAC) {
			favCount++
		}

		if onlineOnly {
			// Show if device is active OR if it is in favorites
			if h.Active || (b.favorites != nil && b.favorites.IsFavorite(h.MAC)) {
				displayedHosts = append(displayedHosts, h)
			}
		} else {
			displayedHosts = append(displayedHosts, h)
		}
	}

	// Sort order:
	// 1. Favorites first (⭐)
	// 2. Active (online) before inactive (offline)
	// 3. Blocked status
	// 4. DisplayName alphabetically
	sort.Slice(displayedHosts, func(i, j int) bool {
		favI := b.favorites != nil && b.favorites.IsFavorite(displayedHosts[i].MAC)
		favJ := b.favorites != nil && b.favorites.IsFavorite(displayedHosts[j].MAC)
		if favI != favJ {
			return favI // true comes before false
		}
		if displayedHosts[i].Active != displayedHosts[j].Active {
			return displayedHosts[i].Active // online before offline
		}
		if displayedHosts[i].IsBlocked() != displayedHosts[j].IsBlocked() {
			return displayedHosts[i].IsBlocked()
		}
		return strings.ToLower(displayedHosts[i].DisplayName()) < strings.ToLower(displayedHosts[j].DisplayName())
	})

	var sb strings.Builder
	if onlineOnly {
		sb.WriteString("📱 <b>Подключенные и избранные устройства</b>\n\n")
		sb.WriteString(fmt.Sprintf("В сети: <b>%d</b>", activeCount))
	} else {
		sb.WriteString("📱 <b>Все зарегистрированные устройства</b>\n\n")
		sb.WriteString(fmt.Sprintf("В сети: <b>%d</b> / Всего: <b>%d</b>", activeCount, len(hotspot.Host)))
	}
	if favCount > 0 {
		sb.WriteString(fmt.Sprintf(" (⭐ Избранных: %d)", favCount))
	}
	if blockedCount > 0 {
		sb.WriteString(fmt.Sprintf(" (🚫 Заблокировано: %d)", blockedCount))
	}
	sb.WriteString("\n<i>Выберите устройство для управления:</i>")

	inlineMarkup := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, h := range displayedHosts {
		statusIcon := "🟢"
		if !h.Active {
			statusIcon = "⚪"
		}
		if h.IsBlocked() {
			statusIcon = "🔴"
		}

		favIcon := ""
		if b.favorites != nil && b.favorites.IsFavorite(h.MAC) {
			favIcon = "⭐ "
		}

		// Device title: Priority is Custom Name, then Hostname, then MAC
		title := fmt.Sprintf("%s%s %s", favIcon, statusIcon, h.DisplayName())
		if h.IP != "" && h.IP != "0.0.0.0" {
			title += fmt.Sprintf(" (%s)", h.IP)
		}
		if h.TrafficShape.Rx > 0 {
			title += fmt.Sprintf(" [⏳%dM]", h.TrafficShape.Rx/1024)
		}

		btn := inlineMarkup.Data(title, "dev_show", h.MAC)
		rows = append(rows, inlineMarkup.Row(btn))
	}

	// Bottom navigation row
	var bottomButtons []tele.Btn
	if onlineOnly {
		bottomButtons = append(bottomButtons, inlineMarkup.Data("⚪ Показать все", "dev_list", "all"))
		bottomButtons = append(bottomButtons, inlineMarkup.Data("⏱ По времени", "dev_text"))
		bottomButtons = append(bottomButtons, inlineMarkup.Data("🔄 Обновить", "dev_list", "online"))
	} else {
		bottomButtons = append(bottomButtons, inlineMarkup.Data("🟢 Только в сети", "dev_list", "online"))
		bottomButtons = append(bottomButtons, inlineMarkup.Data("⏱ По времени", "dev_text"))
		bottomButtons = append(bottomButtons, inlineMarkup.Data("🔄 Обновить", "dev_list", "all"))
	}
	rows = append(rows, inlineMarkup.Row(bottomButtons...))
	inlineMarkup.Inline(rows...)

	if isEdit {
		return b.editOrIgnore(c, sb.String(), inlineMarkup, "Список обновлен")
	}
	return c.Send(sb.String(), sendOptions(inlineMarkup))
}

// handleDeviceCard displays details and controls for a specific host.
func (b *Bot) handleDeviceCard(c tele.Context, mac string, isEdit bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hotspot, err := b.client.GetHotspot(ctx)
	if err != nil {
		log.Printf("[BOT] Error getting hotspot for device %s: %v", mac, err)
		_ = c.Respond(&tele.CallbackResponse{Text: "Ошибка загрузки данных", ShowAlert: true})
		return nil
	}

	var found *keenetic.Host
	for _, h := range hotspot.Host {
		if strings.EqualFold(h.MAC, mac) {
			found = &h
			break
		}
	}

	if found == nil {
		_ = c.Respond(&tele.CallbackResponse{Text: "Устройство не найдено", ShowAlert: true})
		return b.handleDevicesList(c, true, isEdit)
	}

	isFav := b.favorites != nil && b.favorites.IsFavorite(found.MAC)

	var sb strings.Builder
	favPrefix := ""
	if isFav {
		favPrefix = "⭐ "
	}
	sb.WriteString(fmt.Sprintf("💻 <b>Карточка устройства:</b>\n<b>%s%s</b>\n\n", favPrefix, escapeHTML(found.DisplayName())))

	if isFav {
		sb.WriteString("• <b>Избранное:</b> ⭐ Да\n")
	}
	if found.Hostname != "" && found.Hostname != found.DisplayName() {
		sb.WriteString(fmt.Sprintf("• <b>Сетевое имя:</b> <code>%s</code>\n", escapeHTML(found.Hostname)))
	}
	sb.WriteString(fmt.Sprintf("• <b>MAC-адрес:</b> <code>%s</code>\n", found.MAC))
	if found.IP != "" && found.IP != "0.0.0.0" {
		sb.WriteString(fmt.Sprintf("• <b>IP-адрес:</b> <code>%s</code>\n", found.IP))
	}

	statusText := "⚪ Не в сети"
	if found.Active {
		statusText = fmt.Sprintf("🟢 В сети (uptime: %s)", keenetic.FormatDuration(found.Uptime))
	}
	sb.WriteString(fmt.Sprintf("• <b>Статус:</b> %s\n", statusText))

	// Internet access policy
	accessText := "🟢 Разрешен"
	if found.IsBlocked() {
		accessText = "🔴 <b>Заблокирован</b> (No Internet Access)"
	}
	sb.WriteString(fmt.Sprintf("• <b>Доступ в интернет:</b> %s\n", accessText))

	// Speed limit
	speedText := "🚀 Без ограничений"
	if found.TrafficShape.Rx > 0 {
		speedText = fmt.Sprintf("⏳ <b>%d Мбит/с</b> (%d Кбит/с)", found.TrafficShape.Rx/1024, found.TrafficShape.Rx)
	}
	sb.WriteString(fmt.Sprintf("• <b>Ограничение скорости:</b> %s\n", speedText))

	// Traffic stats
	if found.RxBytes > 0 || found.TxBytes > 0 {
		sb.WriteString(fmt.Sprintf("• <b>Трафик:</b> 📥 %s / 📤 %s\n",
			keenetic.FormatBytes(found.RxBytes), keenetic.FormatBytes(found.TxBytes)))
	}

	inlineMarkup := &tele.ReplyMarkup{}
	var rows []tele.Row

	// Row 1: Access toggle
	var accessBtn tele.Btn
	if found.IsBlocked() {
		accessBtn = inlineMarkup.Data("✅ Разрешить интернет", "dev_access", found.MAC, "permit")
	} else {
		accessBtn = inlineMarkup.Data("🚫 Заблокировать интернет", "dev_access", found.MAC, "deny")
	}
	rows = append(rows, inlineMarkup.Row(accessBtn))

	// Row 2: Speed limit & Wake-on-LAN
	speedBtn := inlineMarkup.Data("📶 Скорость", "dev_speed_menu", found.MAC)
	wolBtn := inlineMarkup.Data("⚡ Wake on LAN", "dev_wol", found.MAC)
	rows = append(rows, inlineMarkup.Row(speedBtn, wolBtn))

	// Row 3: Favorite toggle
	var favBtn tele.Btn
	if isFav {
		favBtn = inlineMarkup.Data("★ Убрать из избранного", "dev_fav", found.MAC)
	} else {
		favBtn = inlineMarkup.Data("⭐ В избранное", "dev_fav", found.MAC)
	}
	rows = append(rows, inlineMarkup.Row(favBtn))

	// Row 4: Refresh & Back
	refreshBtn := inlineMarkup.Data("🔄 Обновить", "dev_show", found.MAC)
	backBtn := inlineMarkup.Data("◀️ К списку устройств", "dev_list", "online")
	rows = append(rows, inlineMarkup.Row(refreshBtn, backBtn))

	inlineMarkup.Inline(rows...)

	if isEdit {
		return b.editOrIgnore(c, sb.String(), inlineMarkup, "Данные актуальны")
	}
	return c.Send(sb.String(), sendOptions(inlineMarkup))
}

// handleFavoriteToggle toggles favorite status of device.
func (b *Bot) handleFavoriteToggle(c tele.Context, mac string) error {
	if b.favorites == nil {
		return c.Respond(&tele.CallbackResponse{Text: "Хранилище избранного недоступно"})
	}

	isFav, err := b.favorites.Toggle(mac, "")
	if err != nil {
		log.Printf("[BOT] Error toggling favorite for %s: %v", mac, err)
		return c.Respond(&tele.CallbackResponse{
			Text:      fmt.Sprintf("❌ Ошибка: %v", err),
			ShowAlert: true,
		})
	}

	toast := "⭐ Добавлено в избранное!"
	if !isFav {
		toast = "Удалено из избранного"
	}
	_ = c.Respond(&tele.CallbackResponse{Text: toast})

	return b.handleDeviceCard(c, mac, true)
}

// handleDeviceAccessToggle toggles permit/deny policy for a host.
func (b *Bot) handleDeviceAccessToggle(c tele.Context, mac string, permit bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := b.client.SetDeviceAccess(ctx, mac, permit)
	if err != nil {
		log.Printf("[BOT] Error setting access for %s: %v", mac, err)
		return c.Respond(&tele.CallbackResponse{
			Text:      fmt.Sprintf("❌ Ошибка: %v", err),
			ShowAlert: true,
		})
	}

	statusText := "🔴 Доступ заблокирован!"
	if permit {
		statusText = "🟢 Доступ разрешен!"
	}
	_ = c.Respond(&tele.CallbackResponse{Text: statusText})

	return b.handleDeviceCard(c, mac, true)
}

// handleDeviceWOL sends Wake-on-LAN packet.
func (b *Bot) handleDeviceWOL(c tele.Context, mac string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := b.client.WakeOnLAN(ctx, mac)
	if err != nil {
		log.Printf("[BOT] Error sending WOL to %s: %v", mac, err)
		return c.Respond(&tele.CallbackResponse{
			Text:      fmt.Sprintf("❌ Ошибка отправки WoL: %v", err),
			ShowAlert: true,
		})
	}

	return c.Respond(&tele.CallbackResponse{
		Text:      fmt.Sprintf("⚡ WoL-пакет успешно отправлен на %s!", mac),
		ShowAlert: true,
	})
}

// handleSpeedMenu renders speed limit presets for a host.
func (b *Bot) handleSpeedMenu(c tele.Context, mac string) error {
	inlineMarkup := &tele.ReplyMarkup{}

	btnUnlimited := inlineMarkup.Data("🚀 Без ограничений", "dev_speed_set", mac, "0")

	btn1M := inlineMarkup.Data("1 Мбит/с", "dev_speed_set", mac, "1024")
	btn3M := inlineMarkup.Data("3 Мбит/с", "dev_speed_set", mac, "3072")

	btn5M := inlineMarkup.Data("5 Мбит/с", "dev_speed_set", mac, "5120")
	btn10M := inlineMarkup.Data("10 Мбит/с", "dev_speed_set", mac, "10240")

	btn20M := inlineMarkup.Data("20 Мбит/с", "dev_speed_set", mac, "20480")
	btn50M := inlineMarkup.Data("50 Мбит/с", "dev_speed_set", mac, "51200")

	btnBack := inlineMarkup.Data("◀️ Назад к устройству", "dev_show", mac)

	inlineMarkup.Inline(
		inlineMarkup.Row(btnUnlimited),
		inlineMarkup.Row(btn1M, btn3M),
		inlineMarkup.Row(btn5M, btn10M),
		inlineMarkup.Row(btn20M, btn50M),
		inlineMarkup.Row(btnBack),
	)

	text := fmt.Sprintf(
		"📶 <b>Ограничение скорости интернета</b>\n\n"+
			"Устройство: <code>%s</code>\n\n"+
			"Выберите лимит скорости входящего и исходящего трафика:",
		mac,
	)

	return b.editOrIgnore(c, text, inlineMarkup, "")
}

// handleSpeedSet applies selected speed limit.
func (b *Bot) handleSpeedSet(c tele.Context, mac string, rateKbps int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := b.client.SetDeviceSpeedLimit(ctx, mac, rateKbps)
	if err != nil {
		log.Printf("[BOT] Error setting speed limit (%d kbps) for %s: %v", rateKbps, mac, err)
		return c.Respond(&tele.CallbackResponse{
			Text:      fmt.Sprintf("❌ Ошибка: %v", err),
			ShowAlert: true,
		})
	}

	responseMsg := "🚀 Ограничение скорости снято!"
	if rateKbps > 0 {
		responseMsg = fmt.Sprintf("⏳ Установлен лимит: %d Мбит/с", rateKbps/1024)
	}
	_ = c.Respond(&tele.CallbackResponse{Text: responseMsg})

	return b.handleDeviceCard(c, mac, true)
}

// handleInternetStatus displays internet/WAN/VPN connections and their health.
func (b *Bot) handleInternetStatus(c tele.Context, isEdit bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ifaces, err := b.client.GetInterfaces(ctx)
	if err != nil {
		log.Printf("[BOT] Error getting interfaces: %v", err)
		msg := fmt.Sprintf("❌ Ошибка при получении статуса интернета: %v", err)
		if isEdit {
			return b.editOrIgnore(c, msg, nil, "Ошибка получения данных")
		}
		return c.Send(msg)
	}

	var internetIfaces []keenetic.Interface
	for _, iface := range ifaces {
		if iface.IsInternetConnection() {
			internetIfaces = append(internetIfaces, iface)
		}
	}

	// Sort: connected first, then by interface name
	sort.Slice(internetIfaces, func(i, j int) bool {
		if internetIfaces[i].IsConnected() != internetIfaces[j].IsConnected() {
			return internetIfaces[i].IsConnected()
		}
		return internetIfaces[i].ID < internetIfaces[j].ID
	})

	var sb strings.Builder
	sb.WriteString("🌐 <b>Состояние интернет-подключений</b>\n\n")

	if len(internetIfaces) == 0 {
		sb.WriteString("<i>Интернет-подключения не обнаружены.</i>\n")
	} else {
		for _, iface := range internetIfaces {
			name := iface.Description
			if name == "" {
				name = iface.InterfaceName
			}
			if name == "" {
				name = iface.ID
			}

			icon := "🔴"
			statusText := "Отключен"
			if iface.IsConnected() {
				icon = "🟢"
				statusText = "Подключен"
			}
			if iface.Fail != nil && *iface.Fail != "no" {
				icon = "⚠️"
				statusText = fmt.Sprintf("Ошибка: %s", *iface.Fail)
			}

			sb.WriteString(fmt.Sprintf("%s <b>%s</b> (%s)\n", icon, escapeHTML(name), iface.Type))
			sb.WriteString(fmt.Sprintf("• Статус: <code>%s</code>\n", statusText))

			if iface.Address != nil && *iface.Address != "" && *iface.Address != "0.0.0.0" {
				sb.WriteString(fmt.Sprintf("• IP: <code>%s</code>\n", *iface.Address))
			}
			if iface.Uptime != nil && *iface.Uptime > 0 {
				sb.WriteString(fmt.Sprintf("• Аптайм: %s\n", keenetic.FormatDuration(*iface.Uptime)))
			}
			sb.WriteString("\n")
		}
	}

	inlineMarkup := &tele.ReplyMarkup{}
	btnRefresh := inlineMarkup.Data("🔄 Обновить статус", "inet_refresh")
	inlineMarkup.Inline(inlineMarkup.Row(btnRefresh))

	if isEdit {
		return b.editOrIgnore(c, sb.String(), inlineMarkup, "Статус обновлен")
	}
	return c.Send(sb.String(), sendOptions(inlineMarkup))
}

// handleRebootPrompt asks for confirmation before rebooting.
func (b *Bot) handleRebootPrompt(c tele.Context, isEdit bool) error {
	inlineMarkup := &tele.ReplyMarkup{}
	btnConfirm := inlineMarkup.Data("⚠️ Да, перезагрузить", "reboot_confirm")
	btnCancel := inlineMarkup.Data("❌ Отмена", "reboot_cancel")
	inlineMarkup.Inline(inlineMarkup.Row(btnConfirm, btnCancel))

	text := "⚠️ <b>Внимание! Перезагрузка роутера</b>\n\n" +
		"Вы действительно хотите перезагрузить Keenetic?\n" +
		"Все интернет-соединения и локальная сеть будут временно недоступны в течение 1-2 минут."

	if isEdit {
		return b.editOrIgnore(c, text, inlineMarkup, "")
	}
	return c.Send(text, sendOptions(inlineMarkup))
}

// handleRebootExecute executes the router reboot.
func (b *Bot) handleRebootExecute(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := b.client.Reboot(ctx)
	if err != nil {
		log.Printf("[BOT] Error rebooting router: %v", err)
		return c.Respond(&tele.CallbackResponse{
			Text:      fmt.Sprintf("❌ Ошибка при отправке команды: %v", err),
			ShowAlert: true,
		})
	}

	_ = c.Respond(&tele.CallbackResponse{Text: "Команда на перезагрузку отправлена!"})
	return c.Edit("🔄 <b>Роутер перезагружается...</b>\n\nСвязь восстановится примерно через 1-2 минуты.", &tele.SendOptions{ParseMode: tele.ModeHTML})
}

// handleRebootCancel cancels reboot dialog.
func (b *Bot) handleRebootCancel(c tele.Context) error {
	_ = c.Respond(&tele.CallbackResponse{Text: "Перезагрузка отменена"})
	return c.Edit("❌ Перезагрузка роутера отменена.")
}

// handleSystemInfo displays Keenetic CPU, RAM, and uptime.
func (b *Bot) handleSystemInfo(c tele.Context, isEdit bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sys, err := b.client.GetSystemInfo(ctx)
	if err != nil {
		log.Printf("[BOT] Error getting system info: %v", err)
		msg := fmt.Sprintf("❌ Ошибка при получении информации о системе: %v", err)
		if isEdit {
			return b.editOrIgnore(c, msg, nil, "Ошибка получения данных")
		}
		return c.Send(msg)
	}

	var uptimeStr string
	if uptimeSec, err := strconv.ParseInt(sys.Uptime, 10, 64); err == nil {
		uptimeStr = keenetic.FormatDuration(uptimeSec)
	} else {
		uptimeStr = sys.Uptime
	}

	var sb strings.Builder
	sb.WriteString("ℹ️ <b>Информация о роутере Keenetic</b>\n\n")
	sb.WriteString(fmt.Sprintf("• <b>Модель / Имя:</b> <code>%s</code>\n", escapeHTML(sys.Hostname)))
	sb.WriteString(fmt.Sprintf("• <b>Рабочая группа:</b> <code>%s</code>\n", escapeHTML(sys.Domainname)))
	sb.WriteString(fmt.Sprintf("• <b>Загрузка процессора (CPU):</b> <code>%d%%</code>\n", sys.CPULoad))
	sb.WriteString(fmt.Sprintf("• <b>Использование памяти (RAM):</b> <code>%s</code> (всего: %s, свободно: %s)\n",
		sys.Memory, keenetic.FormatBytes(sys.MemTotal*1024), keenetic.FormatBytes(sys.MemFree*1024)))
	sb.WriteString(fmt.Sprintf("• <b>Время непрерывной работы (Uptime):</b> %s\n", uptimeStr))

	inlineMarkup := &tele.ReplyMarkup{}
	btnRefresh := inlineMarkup.Data("🔄 Обновить", "sys_refresh")
	inlineMarkup.Inline(inlineMarkup.Row(btnRefresh))

	if isEdit {
		return b.editOrIgnore(c, sb.String(), inlineMarkup, "Информация обновлена")
	}
	return c.Send(sb.String(), sendOptions(inlineMarkup))
}

// formatDuration formats a time.Duration into a concise Russian string (e.g. "< 1 мин.", "5 мин.", "1 ч. 20 мин.", "2 д. 4 ч.").
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return "< 1 мин."
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%d д. %d ч.", days, hours)
		}
		return fmt.Sprintf("%d д.", days)
	}
	if hours > 0 {
		if mins > 0 {
			return fmt.Sprintf("%d ч. %d мин.", hours, mins)
		}
		return fmt.Sprintf("%d ч.", hours)
	}
	return fmt.Sprintf("%d мин.", mins)
}

type deviceDurationEntry struct {
	MAC         string
	Name        string
	Active      bool
	Duration    time.Duration
	HasDuration bool
	IsFavorite  bool
}

// handleDevicesActivityText displays devices in plain text:
// 1. Online devices sorted by duration online ascending (fewest minutes first, then longer).
// 2. Offline devices sorted by duration offline ascending (recently went offline first, then longer, then unknown/давно).
func (b *Bot) handleDevicesActivityText(c tele.Context, isEdit bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()

	// Refresh current state from router if available
	hotspot, err := b.client.GetHotspot(ctx)
	if err == nil && hotspot != nil {
		presentMACs := make(map[string]bool, len(hotspot.Host))
		for _, h := range hotspot.Host {
			normMAC := strings.ToLower(strings.TrimSpace(h.MAC))
			presentMACs[normMAC] = true
			if b.history != nil {
				b.history.RecordDevice(h.MAC, h.DisplayName(), h.Active, now)
			}
		}
		if b.history != nil {
			b.history.MarkMissingAsOffline(presentMACs, now)
			_ = b.history.Save()
		}
	} else {
		log.Printf("[BOT] Note: GetHotspot failed (%v), using stored history", err)
	}

	var allItems map[string]storage.DeviceHistoryItem
	if b.history != nil {
		allItems = b.history.GetAll()
	}

	var onlineItems []deviceDurationEntry
	var offlineItems []deviceDurationEntry

	for _, item := range allItems {
		entry := deviceDurationEntry{
			MAC:        item.MAC,
			Name:       item.Name,
			Active:     item.Active,
			IsFavorite: b.favorites != nil && b.favorites.IsFavorite(item.MAC),
		}
		if entry.Name == "" {
			entry.Name = item.MAC
		}

		if item.Active {
			entry.HasDuration = !item.OnlineSince.IsZero()
			if entry.HasDuration {
				entry.Duration = now.Sub(item.OnlineSince)
			}
			onlineItems = append(onlineItems, entry)
		} else {
			entry.HasDuration = !item.OfflineSince.IsZero()
			if entry.HasDuration {
				entry.Duration = now.Sub(item.OfflineSince)
			}
			offlineItems = append(offlineItems, entry)
		}
	}

	// 1. Sort online items: duration online ascending (least time first, then more)
	sort.Slice(onlineItems, func(i, j int) bool {
		if onlineItems[i].Duration != onlineItems[j].Duration {
			return onlineItems[i].Duration < onlineItems[j].Duration
		}
		return strings.ToLower(onlineItems[i].Name) < strings.ToLower(onlineItems[j].Name)
	})

	// 2. Sort offline items: recently offline first (least time first), then longer, then unknown ("давно")
	sort.Slice(offlineItems, func(i, j int) bool {
		if offlineItems[i].HasDuration != offlineItems[j].HasDuration {
			return offlineItems[i].HasDuration // known duration comes before unknown
		}
		if offlineItems[i].HasDuration && offlineItems[j].HasDuration {
			if offlineItems[i].Duration != offlineItems[j].Duration {
				return offlineItems[i].Duration < offlineItems[j].Duration
			}
		}
		return strings.ToLower(offlineItems[i].Name) < strings.ToLower(offlineItems[j].Name)
	})

	var sb strings.Builder
	sb.WriteString("📋 <b>Список устройств по времени активности</b>\n\n")

	sb.WriteString(fmt.Sprintf("🟢 <b>Онлайн (%d):</b>\n", len(onlineItems)))
	if len(onlineItems) == 0 {
		sb.WriteString("<i>Нет устройств в сети</i>\n")
	} else {
		for i, item := range onlineItems {
			fav := ""
			if item.IsFavorite {
				fav = "⭐ "
			}
			durStr := formatDuration(item.Duration)
			sb.WriteString(fmt.Sprintf("%d. %s<b>%s</b> — %s\n", i+1, fav, escapeHTML(item.Name), durStr))
		}
	}

	sb.WriteString(fmt.Sprintf("\n🔴 <b>Оффлайн (%d):</b>\n", len(offlineItems)))
	if len(offlineItems) == 0 {
		sb.WriteString("<i>Все устройства в сети</i>\n")
	} else {
		for i, item := range offlineItems {
			fav := ""
			if item.IsFavorite {
				fav = "⭐ "
			}
			durStr := "давно"
			if item.HasDuration {
				durStr = formatDuration(item.Duration)
			}
			sb.WriteString(fmt.Sprintf("%d. %s<b>%s</b> — %s\n", i+1, fav, escapeHTML(item.Name), durStr))
		}
	}

	markup := &tele.ReplyMarkup{}
	btnRefresh := markup.Data("🔄 Обновить", "dev_text_refresh")
	btnInteractive := markup.Data("📱 Панель устройств", "dev_list", "online")
	markup.Inline(markup.Row(btnRefresh, btnInteractive))

	msgText := sb.String()
	if isEdit {
		return b.editOrIgnore(c, msgText, markup, "Список обновлен")
	}
	return c.Send(msgText, sendOptions(markup))
}
