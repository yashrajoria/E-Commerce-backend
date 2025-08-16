package kafka

import (
	"context"
	"encoding/json"
	"log"
	"payment-service/models"

	"github.com/segmentio/kafka-go"
)

type PaymentEventProducer struct {
	writer *kafka.Writer
	topic  string
}

func NewPaymentEventProducer(brokers []string, topic string) *PaymentEventProducer {

	w := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	log.Printf("[OrderService][KafkaProducer] initialized topic=%s brokers=%v", topic, brokers)
	return &PaymentEventProducer{writer: w, topic: topic}
}

func (p *PaymentEventProducer) SendPaymentEvent(event models.PaymentEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := kafka.Message{
		Key:   []byte(event.OrderID),
		Value: data,
	}

	if err := p.writer.WriteMessages(context.Background(), msg); err != nil {
		log.Printf("[PaymentService] ❌ Failed to send payment event: %v", err)
		return err
	}

	log.Printf("[PaymentService] 📤 Sent PaymentEvent: %+v", event)
	return nil
}

func (p *PaymentEventProducer) Close() {
	_ = p.writer.Close()
	log.Println("[PaymentService] 🔌 Kafka producer closed")
}
