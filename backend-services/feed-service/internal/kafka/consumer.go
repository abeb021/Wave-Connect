package kafka

import (
	"context"
	"encoding/json"
	"log"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/abeb021/wave-connect/backend-services/pkg/metrics"
)

type Consumer struct {
	kr   *kafka.Consumer
	repo ProfileRepository
}

// dependency inversion
// kafka should not be strongly connected with repo
// the only thing we need from repo is this 3 methods
type ProfileRepository interface {
	UpsertProfile(ctx context.Context, userID, username, bio string, avatar []byte) error
	IsEventProcessed(ctx context.Context, eventID string) (bool, error)
	MarkEventProcessed(ctx context.Context, eventID, eventType string) error
}

type ProfileUpdatedEvent struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Bio      string `json:"bio"`
	Avatar   []byte `json:"avatar"`
}

func NewConsumer(broker, groupID string, repo ProfileRepository) (*Consumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  broker,
		"group.id":           groupID,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
	})
	if err != nil {
		return nil, err
	}

	return &Consumer{kr: c, repo: repo}, nil
}

func (c *Consumer) Close() {
	c.kr.Close()
}

func (c *Consumer) Start(ctx context.Context, topics []string) error {
	err := c.kr.SubscribeTopics(topics, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("consumer shutting down")
			return ctx.Err()
		default:
			ev := c.kr.Poll(100)
			if ev == nil {
				continue
			}

			switch e := ev.(type) {
			case *kafka.Message:
				c.handleMessage(e)
			case kafka.Error:
				log.Printf("consumer error: %v\n", e)
			default:
				log.Printf("unknown event type: %v\n", e)
			}
		}
	}
}

func (c *Consumer) handleMessage(msg *kafka.Message) {
	var event ProfileUpdatedEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		log.Printf("failed to unmarshall event: %v\n", err)
		return
	}

	eventID := msg.TopicPartition.String()

	processed, err := c.repo.IsEventProcessed(context.Background(), eventID)
	if err != nil {
		log.Printf("failed to check event: %v\n", err)
		return
	}
	if processed {
		return
	}

	if err := c.repo.UpsertProfile(context.Background(), event.UserID, event.Username, event.Bio, event.Avatar); err != nil {
		metrics.KafkaEventsProcessed.WithLabelValues("feed", "profile-updates", "failed").Inc()
		log.Printf("failed to update profile: %v\n", err)
		return
	}

	if err := c.repo.MarkEventProcessed(context.Background(), eventID, "profile.updated"); err != nil {
		metrics.KafkaEventsProcessed.WithLabelValues("feed", "profile-updates", "failed").Inc()
		log.Printf("failed to update mark event: %v\n", err)
		return
	}

	metrics.KafkaEventsProcessed.WithLabelValues("feed", "profile-updates", "processed").Inc()

	if _, err := c.kr.CommitMessage(msg); err != nil {
		log.Printf("failed to commit offset: %v\n", err)
	}
}
