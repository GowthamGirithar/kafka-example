package main

import (
	"flag"
	"fmt"
	sar "github.com/IBM/sarama"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

var (
	// kafka-example
	kafkaBrokerUrl     string
	kafkaVerbose       bool
	kafkaTopic         string
	kafkaConsumerGroup string
	kafkaClientId      string
)

func main() {
	flag.StringVar(&kafkaBrokerUrl, "kafka-brokers", "<my ip address>:29093,<my ip address>:19093", "Kafka brokers in comma separated value")
	flag.BoolVar(&kafkaVerbose, "kafka-example-verbose", true, "Kafka verbose logging")
	flag.StringVar(&kafkaTopic, "kafka-topic", "tettttt", "Kafka topic. Only one topic per worker.")
	flag.StringVar(&kafkaConsumerGroup, "kafka-consumer-group", "consumer-group", "Kafka consumer group")
	flag.StringVar(&kafkaClientId, "kafka-client-id", "my-client-id", "Kafka client id")

	flag.Parse()

	cfg := sar.NewConfig()

	adminClient(cfg) // create topic
	go produceMessage(cfg)
	consumeMessage(cfg)

}

func produceMessage(cfg *sar.Config) {
	brokers := strings.Split(kafkaBrokerUrl, ",")
	cfg.Producer.Return.Successes = true
	prod, err := sar.NewSyncProducer(brokers, cfg)
	if err != nil {
		fmt.Println("Error creating producer", err)
	}
	defer prod.Close()

	// Produce messages
	for i := 0; i < 50; i++ {
		message := fmt.Sprintf("Message %d - %s", i, time.Now().String())
		msg := &sar.ProducerMessage{
			Topic: kafkaTopic,
			Value: sar.StringEncoder(message),
		}

		// Send message
		partition, offset, err := prod.SendMessage(msg)
		if err != nil {
			fmt.Printf("Failed to send message: %s\n", err)
		} else {
			fmt.Printf("Message sent successfully, partition: %d, offset: %d\n", partition, offset)
		}
		time.Sleep(1 * time.Second) // Wait 1 second between messages
	}
}

func adminClient(cfg *sar.Config) {
	brokers := strings.Split(kafkaBrokerUrl, ",")
	// Create the Kafka admin client
	adminClient, err := sar.NewClusterAdmin(brokers, cfg)
	if err != nil {
		fmt.Println("Error creating admin client:", err)
		return
	}
	defer adminClient.Close()

	// Create the topic
	topicDetail := &sar.TopicDetail{
		NumPartitions:     3, // Number of partitions
		ReplicationFactor: 2, // Replication factor
	}
	err = adminClient.CreateTopic(kafkaTopic, topicDetail, false)
	if err != nil {
		fmt.Println("Error creating topic:", err)
		return
	}
	fmt.Println("Topic created successfully")
}

func consumeMessage(cfg *sar.Config) {
	brokers := strings.Split(kafkaBrokerUrl, ",")
	consumer, err := sar.NewConsumer(brokers, cfg)
	if err != nil {
		fmt.Println("error in creating new consumers", err)
		return
	}
	defer consumer.Close()

	// Get the list of partitions for the topic
	partitions, err := consumer.Partitions(kafkaTopic)
	if err != nil {
		fmt.Printf("Error getting partitions: %v\n", err)
		return
	}

	var wg sync.WaitGroup

	// Handle signals to gracefully shut down consumer
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	// Consume messages from each partition
	for i, partition := range partitions {
		//OffsetNewest - will read from the latest offset
		// OffsetOldest - read from old offset
		partitionConsumer, err := consumer.ConsumePartition(kafkaTopic, partition, sar.OffsetNewest)
		if err != nil {
			fmt.Printf("Error creating partition consumer for partition %d: %v\n", partition, err)
			return
		}
		defer partitionConsumer.Close()

		wg.Add(1)
		go func(partitionNum int, pc sar.PartitionConsumer) {
			defer wg.Done()
			for {
				select {
				case msg := <-pc.Messages():
					fmt.Printf("Partition %d - Received message: %s\n", partitionNum, string(msg.Value))
				case err := <-pc.Errors():
					fmt.Printf("Partition %d - Error consuming message: %v\n", partitionNum, err)
				case <-signals:
					return
				}
			}
		}(i, partitionConsumer)
	}

	// Wait for all partition consumers to finish
	wg.Wait()
}
