package kafka

import (
	"context"
	"encoding/json"
	"log"
	"order-service/models"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
	topic  string
}

func NewProducer(brokers []string, topic string) *Producer {
	w := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	log.Printf("[OrderService][KafkaProducer] initialized topic=%s brokers=%v", topic, brokers)
	return &Producer{writer: w, topic: topic}
}

func (p *Producer) Publish(topic string, message []byte) error {
	msg := kafka.Message{
		Topic: topic,
		Value: message,
	}
	return p.writer.WriteMessages(context.Background(), msg)
}

func (p *Producer) SendPaymentRequest(evt models.PaymentRequest) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	msg := kafka.Message{
		Key:   []byte(evt.OrderID),
		Value: data,
	}
	if err := p.writer.WriteMessages(context.Background(), msg); err != nil {
		log.Printf("❌ [OrderService][KafkaProducer] failed to publish payment-request order=%s topic=%s err=%v", evt.OrderID, p.topic, err)
		return err
	}
	log.Printf("✅ [OrderService][KafkaProducer] payment-request published order=%s amount=%d topic=%s", evt.OrderID, evt.Amount, p.topic)
	return nil
}

func (p *Producer) Close() error {
	log.Printf("[OrderService][KafkaProducer] closing writer topic=%s", p.topic)
	return p.writer.Close()
}
