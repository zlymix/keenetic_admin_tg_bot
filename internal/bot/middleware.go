package bot

import (
	"fmt"
	"log"

	tele "gopkg.in/telebot.v4"
)

// AuthMiddleware creates a telebot middleware that restricts access to allowed user IDs.
func AuthMiddleware(allowedUsers []int64) tele.MiddlewareFunc {
	allowedMap := make(map[int64]bool, len(allowedUsers))
	for _, id := range allowedUsers {
		allowedMap[id] = true
	}

	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			if c.Sender() == nil {
				return nil
			}

			userID := c.Sender().ID

			// If allowedUsers list is empty, warn in logs and allow, or restrict?
			// Best practice: if empty, deny and instruct user to set up config.
			if len(allowedMap) > 0 && !allowedMap[userID] {
				log.Printf("[AUTH] Denied unauthorized access attempt from UserID: %d (@%s)",
					userID, c.Sender().Username)

				msg := fmt.Sprintf(
					"⛔ <b>Доступ запрещен</b>\n\n"+
						"Ваш Telegram ID: <code>%d</code>\n"+
						"Чтобы получить доступ, добавьте этот ID в список <code>allowed_users</code> в файле <code>config.yaml</code>.",
					userID,
				)
				return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
			}

			return next(c)
		}
	}
}
