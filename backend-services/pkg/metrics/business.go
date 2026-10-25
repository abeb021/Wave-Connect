package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// 1. Успешные логины (auth-service)
	AuthLoginsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wave_auth_logins_total",
			Help: "Total number of login attempts by status",
		},
		[]string{"status"}, // "success" | "failed"
	)

	// 2. Созданные публикации (feed-service)
	FeedPublicationsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "wave_feed_publications_total",
			Help: "Total number of publications created",
		},
	)

	// 3. Отправленные сообщения (chat-service)
	ChatMessagesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "wave_chat_messages_total",
			Help: "Total number of chat messages sent",
		},
	)

	// 4. Обновления профиля (profile-service)
	ProfileUpdatesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "wave_profile_updates_total",
			Help: "Total number of profile updates",
		},
	)
)

func InitBusinessMetrics() {
	prometheus.MustRegister(AuthLoginsTotal)
	prometheus.MustRegister(FeedPublicationsTotal)
	prometheus.MustRegister(ChatMessagesTotal)
	prometheus.MustRegister(ProfileUpdatesTotal)
}