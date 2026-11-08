package kafka

import (
	"log"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type Producer struct {
	kp *kafka.Producer
}

func NewProducer(broker string) (*Producer, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": broker,
	})
	if err != nil {
		return nil, err
	}

	go func() {
		for e := range p.Events() {
			switch ev := e.(type) {
			//message is an error of a C kafka
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					log.Printf("Delivery failed: %v\n", ev.TopicPartition.Error)
				} else {
					log.Printf("Message delivered: %v\n", ev.TopicPartition)
				}
			//this is an error of a producer (lib)
			case kafka.Error:
				log.Printf("kafka error: %v\n", err)
			}
		}
	}()

	return &Producer{kp: p}, nil
}

func (p *Producer) Close() {
	//wait 10 sec
	p.kp.Flush(10 * 1000)
	p.kp.Close()
}

func (p *Producer) Send(topic, key string, value []byte) error {
	err := p.kp.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Key:   []byte(key),
		Value: value,
	}, nil)
	//chan is nil so it uses p.kp.Events() as a default channel

	return err
}
