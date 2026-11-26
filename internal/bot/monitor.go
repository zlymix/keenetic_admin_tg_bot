package bot

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"keenetic_admin/internal/keenetic"

	tele "gopkg.in/telebot.v4"
)

type DeviceMonitor struct {
	client   *keenetic.Client
	bot      *tele.Bot
	chatID   int64
	interval time.Duration

	mu              sync.RWMutex
	lastActiveHosts map[string]keenetic.Host
	knownNames      map[string]string
	initialized     bool
}

func NewDeviceMonitor(client *keenetic.Client, bot *tele.Bot, chatID int64, interval time.Duration) *DeviceMonitor {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &DeviceMonitor{
		client:          client,
		bot:             bot,
		chatID:          chatID,
		interval:        interval,
		lastActiveHosts: make(map[string]keenetic.Host),
		knownNames:      make(map[string]string),
	}
}

func (m *DeviceMonitor) Start(ctx context.Context) {
	if m.chatID == 0 {
		log.Println("[MONITOR] Notification chat ID is 0; device connect/disconnect notifications are disabled.")
		return
	}

	log.Printf("[MONITOR] Starting device connect/disconnect monitor (interval: %v, chatID: %d)", m.interval, m.chatID)

	// Initial poll immediately
	m.poll(ctx)

	ticker := time.NewTicker(m.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("[MONITOR] Device monitor stopped.")
				return
			case <-ticker.C:
				m.poll(ctx)
			}
		}
	}()
}

func (m *DeviceMonitor) poll(ctx context.Context) {
	pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	hotspot, err := m.client.GetHotspot(pollCtx)
	if err != nil {
		log.Printf("[MONITOR] Warning: failed to fetch hotspot data: %v", err)
		// Don't wipe active devices on network error to avoid false disconnect alarms
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	currentActive := make(map[string]keenetic.Host)

	for _, h := range hotspot.Host {
		// Update known display names
		displayName := h.DisplayName()
		m.knownNames[h.MAC] = displayName

		if h.Active {
			currentActive[h.MAC] = h
		}
	}

	// If this is the very first poll, initialize the baseline without sending messages
	if !m.initialized {
		m.lastActiveHosts = currentActive
		m.initialized = true
		log.Printf("[MONITOR] Initial baseline recorded with %d active devices.", len(currentActive))
		return
	}

	nowStr := time.Now().Format("15:04:05")

	// Detect new connections
	for mac, host := range currentActive {
		if _, existed := m.lastActiveHosts[mac]; !existed {
			name := host.DisplayName()
			ip := host.IP
			if ip == "" {
				ip = "—"
			}
			msg := fmt.Sprintf(
				"🟢 <b>Устройство подключилось к сети</b>\n\n"+
					"📱 <b>%s</b>\n"+
					"🌐 IP: <code>%s</code>\n"+
					"🏷 MAC: <code>%s</code>\n"+
					"⏰ Время: %s",
				escapeHTML(name), ip, mac, nowStr,
			)
			m.notify(msg)
		}
	}

	// Detect disconnections
	for mac, host := range m.lastActiveHosts {
		if _, stillActive := currentActive[mac]; !stillActive {
			name := host.DisplayName()
			if savedName, ok := m.knownNames[mac]; ok && savedName != "" {
				name = savedName
			}
			ip := host.IP
			if ip == "" {
				ip = "—"
			}
			msg := fmt.Sprintf(
				"🔴 <b>Устройство отключилось от сети</b>\n\n"+
					"📱 <b>%s</b>\n"+
					"🌐 IP: <code>%s</code>\n"+
					"🏷 MAC: <code>%s</code>\n"+
					"⏰ Время: %s",
				escapeHTML(name), ip, mac, nowStr,
			)
			m.notify(msg)
		}
	}

	m.lastActiveHosts = currentActive
}

func (m *DeviceMonitor) notify(text string) {
	recipient := &tele.Chat{ID: m.chatID}
	go func() {
		_, err := m.bot.Send(recipient, text, &tele.SendOptions{ParseMode: tele.ModeHTML})
		if err != nil {
			log.Printf("[MONITOR] Error sending alert to chat %d: %v", m.chatID, err)
		}
	}()
}
