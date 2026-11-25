package producer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/linkedin/goavro/v2"
	"github.com/twmb/franz-go/pkg/kgo"
)

const topic = "movie-events"

type Producer struct {
	kafka    *kgo.Client
	codec    *goavro.Codec
	schemaID uint32
}

func New(brokers []string, registryURL string) (*Producer, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(registryURL + "/subjects/" + topic + "-value/versions/1")
	if err != nil {
		return nil, fmt.Errorf("load Avro schema: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("load Avro schema: registry returned %s", resp.Status)
	}
	var schema struct {
		ID     uint32 `json:"id"`
		Schema string `json:"schema"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		return nil, fmt.Errorf("read Avro schema: %w", err)
	}
	codec, err := goavro.NewCodec(schema.Schema)
	if err != nil {
		return nil, fmt.Errorf("parse Avro schema: %w", err)
	}
	kafka, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordRetries(5),
		kgo.RecordDeliveryTimeout(15*time.Second),
		kgo.RetryBackoffFn(func(attempt int) time.Duration {
			return (250 * time.Millisecond) << min(attempt, 4)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka client: %w", err)
	}
	return &Producer{kafka: kafka, codec: codec, schemaID: schema.ID}, nil
}

func (p *Producer) Close() {
	p.kafka.Close()
}

func (p *Producer) encode(event Event) ([]byte, error) {
	if err := event.validate(); err != nil {
		return nil, err
	}
	prefix := make([]byte, 5)
	binary.BigEndian.PutUint32(prefix[1:], p.schemaID)
	return p.codec.BinaryFromNative(prefix, event.avroRecord())
}

func (p *Producer) publish(ctx context.Context, event Event, value []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err := p.kafka.ProduceSync(ctx, &kgo.Record{
		Topic: topic,
		Key:   []byte(event.UserID),
		Value: value,
	}).FirstErr()
	if err != nil {
		return err
	}
	slog.Info("event published", "event_id", event.ID, "event_type", event.Type, "timestamp", event.Timestamp.UTC())
	return nil
}
