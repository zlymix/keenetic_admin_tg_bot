package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"keenetic_admin/internal/keenetic"
	"keenetic_admin/internal/storage"

	tele "gopkg.in/telebot.v4"
)

type DeviceMonitor struct {
	client   *keenetic.Client
	bot      *tele.Bot
	chatID   int64
	interval time.Duration
	history  *storage.HistoryStorage

	mu          sync.RWMutex
	initialized bool
}

func NewDeviceMonitor(client *keenetic.Client, bot *tele.Bot, chatID int64, interval time.Duration, history *storage.HistoryStorage) *DeviceMonitor {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &DeviceMonitor{
		client:   client,
		bot:      bot,
		chatID:   chatID,
		interval: interval,
		history:  history,
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
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	presentMACs := make(map[string]bool, len(hotspot.Host))
	activeCount := 0

	for _, h := range hotspot.Host {
		normMAC := strings.ToLower(strings.TrimSpace(h.MAC))
		presentMACs[normMAC] = true
		if h.Active {
			activeCount++
		}

		name := h.DisplayName()
		var trans string
		if m.history != nil {
			trans = m.history.RecordDevice(h.MAC, name, h.Active, now)
		}

		if m.initialized {
			if trans == "connected" {
				m.notify(fmt.Sprintf("🟢 <b>%s</b>", escapeHTML(name)))
			} else if trans == "disconnected" {
				m.notify(fmt.Sprintf("🔴 <b>%s</b>", escapeHTML(name)))
			}
		}
	}

	// Detect any devices that disappeared completely from router hotspot
	if m.history != nil {
		missing := m.history.MarkMissingAsOffline(presentMACs, now)
		if m.initialized {
			for _, item := range missing {
				m.notify(fmt.Sprintf("🔴 <b>%s</b>", escapeHTML(item.Name)))
			}
		}
		_ = m.history.Save()
	}

	// Initial baseline on startup
	if !m.initialized {
		m.initialized = true
		log.Printf("[MONITOR] Initial baseline recorded with %d active devices.", activeCount)
	}
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
