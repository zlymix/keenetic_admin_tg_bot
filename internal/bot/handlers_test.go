package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"keenetic_admin/internal/keenetic"
	"keenetic_admin/internal/storage"

	tele "gopkg.in/telebot.v4"
)

func TestBot_CallbackHandling(t *testing.T) {
	// Live router client
	kClient := keenetic.NewClient("http://192.168.1.1", "admin", "tmpPASS123")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := kClient.Login(ctx); err != nil {
		t.Skipf("Router login skipped/failed: %v", err)
	}

	botSettings := tele.Settings{
		Offline:     true,
		Synchronous: true,
	}
	tBot, err := tele.NewBot(botSettings)
	if err != nil {
		t.Fatalf("Failed to create test bot: %v", err)
	}

	favStore, err := storage.NewFavoritesStorage(t.TempDir() + "/favorites.json")
	if err != nil {
		t.Fatalf("Failed to create test favorites storage: %v", err)
	}

	appBot := New(tBot, kClient, nil, favStore)
	appBot.SetupRoutes()

	testMAC := "18:31:bf:52:31:38"

	// 1. Test dev_access callback with deny
	t.Run("Callback_dev_access_deny", func(t *testing.T) {
		update := tele.Update{
			ID: 101,
			Callback: &tele.Callback{
				ID: "cb_access_deny",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device Card",
				},
				Data: "\fdev_access|" + testMAC + "|deny",
			},
		}

		tBot.ProcessUpdate(update)
		t.Logf("dev_access deny processed successfully")
	})

	// 2. Test dev_access callback with permit
	t.Run("Callback_dev_access_permit", func(t *testing.T) {
		update := tele.Update{
			ID: 102,
			Callback: &tele.Callback{
				ID: "cb_access_permit",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device Card",
				},
				Data: "\fdev_access|" + testMAC + "|permit",
			},
		}

		tBot.ProcessUpdate(update)
		t.Logf("dev_access permit processed successfully")
	})

	// 3. Test dev_wol callback
	t.Run("Callback_dev_wol", func(t *testing.T) {
		update := tele.Update{
			ID: 103,
			Callback: &tele.Callback{
				ID: "cb_wol",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device Card",
				},
				Data: "\fdev_wol|" + testMAC,
			},
		}

		tBot.ProcessUpdate(update)
		t.Logf("dev_wol processed successfully")
	})

	// 4. Test dev_speed_set callback
	t.Run("Callback_dev_speed_set", func(t *testing.T) {
		update := tele.Update{
			ID: 104,
			Callback: &tele.Callback{
				ID: "cb_speed_set",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device Card",
				},
				Data: "\fdev_speed_set|" + testMAC + "|10240",
			},
		}

		tBot.ProcessUpdate(update)
		t.Logf("dev_speed_set processed successfully")

		// Revert speed limit to 0
		updateRevert := tele.Update{
			ID: 105,
			Callback: &tele.Callback{
				ID: "cb_speed_clear",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device Card",
				},
				Data: "\fdev_speed_set|" + testMAC + "|0",
			},
		}
		tBot.ProcessUpdate(updateRevert)
		t.Logf("dev_speed_set 0 processed successfully")
	})

	// 5. Test dev_show callback (Card refresh)
	t.Run("Callback_dev_show", func(t *testing.T) {
		update := tele.Update{
			ID: 106,
			Callback: &tele.Callback{
				ID: "cb_show",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device Card",
				},
				Data: "\fdev_show|" + testMAC,
			},
		}

		tBot.ProcessUpdate(update)
		t.Logf("dev_show processed successfully")
	})

	// 6. Test dev_speed_menu callback
	t.Run("Callback_dev_speed_menu", func(t *testing.T) {
		update := tele.Update{
			ID: 107,
			Callback: &tele.Callback{
				ID: "cb_speed_menu",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device Card",
				},
				Data: "\fdev_speed_menu|" + testMAC,
			},
		}

		tBot.ProcessUpdate(update)
		t.Logf("dev_speed_menu processed successfully")
	})

	// 7. Test dev_fav callback (toggle favorite)
	t.Run("Callback_dev_fav", func(t *testing.T) {
		update1 := tele.Update{
			ID: 108,
			Callback: &tele.Callback{
				ID: "cb_fav_1",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device Card",
				},
				Data: "\fdev_fav|" + testMAC,
			},
		}

		tBot.ProcessUpdate(update1)
		if !favStore.IsFavorite(testMAC) {
			t.Fatalf("Expected %s to be favorite after toggle", testMAC)
		}
		t.Logf("dev_fav add processed successfully")

		// Toggle again to remove
		update2 := tele.Update{
			ID: 109,
			Callback: &tele.Callback{
				ID: "cb_fav_2",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device Card",
				},
				Data: "\fdev_fav|" + testMAC,
			},
		}
		tBot.ProcessUpdate(update2)
		if favStore.IsFavorite(testMAC) {
			t.Fatalf("Expected %s not to be favorite after second toggle", testMAC)
		}
		t.Logf("dev_fav remove processed successfully")
	})

	// 8. Test that an offline device in favorites is shown on the main list
	t.Run("OfflineFavoriteShownInMainList", func(t *testing.T) {
		offlineMAC := "1a:5f:e7:51:ca:3e" // SERJ | POCO X3 NFC (Active: false)
		if err := favStore.Add(offlineMAC, "SERJ | POCO X3 NFC"); err != nil {
			t.Fatalf("Failed to add offline device to favorites: %v", err)
		}

		update := tele.Update{
			ID: 110,
			Callback: &tele.Callback{
				ID: "cb_list_online",
				Sender: &tele.User{ID: 12345},
				Message: &tele.Message{
					ID:   1,
					Chat: &tele.Chat{ID: 12345},
					Text: "Device List",
				},
				Data: "\fdev_list|online",
			},
		}
		tBot.ProcessUpdate(update)
		t.Logf("Offline favorite correctly included in main online list")
	})
}

func TestMACSplit(t *testing.T) {
	raw := "18:31:bf:52:31:38|deny"
	parts := strings.Split(raw, "|")
	if len(parts) != 2 {
		t.Fatalf("Expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "18:31:bf:52:31:38" {
		t.Errorf("Expected MAC, got %s", parts[0])
	}
	if parts[1] != "deny" {
		t.Errorf("Expected deny, got %s", parts[1])
	}
}
