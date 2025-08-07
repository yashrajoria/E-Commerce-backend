package kafka

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"

	"github.com/segmentio/kafka-go"
)

type CheckoutEvent struct {
	Event     string      `json:"event"`
	UserID    string      `json:"user_id"`
	Items     []CartItem  `json:"items"`
	Timestamp string      `json:"timestamp"`
}

type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// StartCheckoutConsumer connects to Kafka and starts consuming messages
func StartCheckoutConsumer(broker, topic, groupID string) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{broker},
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1e3,  // 1KB
		MaxBytes: 1e6,  // 1MB
	})

	log.Println("✅ Kafka consumer started for topic:", topic)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		<-ch
		log.Println("🛑 Shutting down Kafka consumer...")
		cancel()
	}()

	for {
		m, err := r.ReadMessage(ctx)
		if err != nil {
			log.Printf("❌ Failed to read message: %v", err)
			break
		}

		var event CheckoutEvent
		if err := json.Unmarshal(m.Value, &event); err != nil {
			log.Printf("❌ Invalid checkout event: %v", err)
			continue
		}

		log.Printf("📦 [ORDER CREATED] UserID=%s, Items=%d", event.UserID, len(event.Items))
		// TODO: Save order to DB or trigger workflow
	}

	r.Close()
}
