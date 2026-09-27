/*
Copyright (c) 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except
in compliance with the License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0
*/

package integration_test

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/IBM/sarama"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/osac-project/osac-metering/internal/events"
	kafkapub "github.com/osac-project/osac-metering/internal/kafka"
	"github.com/osac-project/osac-metering/internal/testutil"
	"github.com/osac-project/osac-metering/internal/watch"
	"github.com/osac-project/osac-metering/schema"
	privatev1 "github.com/osac-project/osac/proto/gen/osac/private/v1"
)

const kafkaTestImage = "confluentinc/confluent-local:7.5.0"

var (
	kafkaContainer *kafka.KafkaContainer
	kafkaBrokers   []string
)

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var err error
	kafkaContainer, err = kafka.Run(ctx, kafkaTestImage, kafka.WithClusterID("osac-metering-integration"))
	Expect(err).NotTo(HaveOccurred())

	kafkaBrokers, err = kafkaContainer.Brokers(ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(kafkaBrokers).NotTo(BeEmpty())
})

var _ = AfterSuite(func() {
	if kafkaContainer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	Expect(kafkaContainer.Terminate(ctx)).To(Succeed())
})

var _ = Describe("fulfillment Watch to Kafka", func() {
	It("publishes the mapped CloudEvent to a real Kafka broker", func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		const topic = kafkapub.TopicLifecycle
		adminConfig := sarama.NewConfig()
		adminConfig.Version = sarama.V3_5_0_0
		admin, err := sarama.NewClusterAdmin(kafkaBrokers, adminConfig)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(admin.Close()).To(Succeed()) })
		Expect(admin.CreateTopic(topic, &sarama.TopicDetail{
			NumPartitions:     1,
			ReplicationFactor: 1,
		}, false)).To(Succeed())

		consumerConfig := sarama.NewConfig()
		consumerConfig.Version = sarama.V3_5_0_0
		consumerConfig.Consumer.Return.Errors = true
		kafkaConsumer, err := sarama.NewConsumer(kafkaBrokers, consumerConfig)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(kafkaConsumer.Close()).To(Succeed()) })

		partition, err := kafkaConsumer.ConsumePartition(topic, 0, sarama.OffsetOldest)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(partition.Close()).To(Succeed()) })

		producerConfig := kafkapub.NewProducerConfig()
		producerConfig.Version = sarama.V3_5_0_0
		producer, err := sarama.NewSyncProducer(kafkaBrokers, producerConfig)
		Expect(err).NotTo(HaveOccurred())
		publisher := kafkapub.NewPublisher(producer)
		DeferCleanup(func() { Expect(publisher.Close()).To(Succeed()) })

		transitionTime := time.Date(2026, time.January, 15, 12, 30, 0, 0, time.UTC)
		eventID := uuid.NewString()
		resourceID := uuid.NewString()
		watchedEvent := &privatev1.Event{
			Id:        eventID,
			Type:      privatev1.EventType_EVENT_TYPE_OBJECT_CREATED,
			Timestamp: timestamppb.New(transitionTime),
			Payload: &privatev1.Event_ComputeInstance{ComputeInstance: &privatev1.ComputeInstance{
				Id: resourceID,
				Metadata: &privatev1.Metadata{
					Tenant:            "tenant-integration",
					Project:           "project-integration",
					Version:           1,
					CreationTimestamp: timestamppb.New(transitionTime),
				},
				Status: &privatev1.ComputeInstanceStatus{
					State:               privatev1.ComputeInstanceState_COMPUTE_INSTANCE_STATE_RUNNING,
					StateTransitionTime: timestamppb.New(transitionTime),
				},
			}},
		}
		watchServer := &watchServerFake{
			event:    watchedEvent,
			requests: make(chan *privatev1.EventsWatchRequest, 1),
		}

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		grpcServer := grpc.NewServer()
		privatev1.RegisterEventsServer(grpcServer, watchServer)
		go func() { _ = grpcServer.Serve(listener) }()
		DeferCleanup(grpcServer.Stop)

		grpcConn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(grpcConn.Close()).To(Succeed()) })

		mapperFactory, err := watch.NewMapperFactory(unusedExternalIPPoolGetter{}, "integration-deployment", map[string]string{})
		Expect(err).NotTo(HaveOccurred())
		store := testutil.NewMonotonicMemoryProjectionStore(nil)
		meteringConsumer, err := watch.NewConsumer(
			privatev1.NewEventsClient(grpcConn),
			publisher,
			store,
			logr.Discard(),
			mapperFactory,
		)
		Expect(err).NotTo(HaveOccurred())

		consumerCtx, cancelConsumer := context.WithCancel(ctx)
		consumerDone := make(chan error, 1)
		go func() { consumerDone <- meteringConsumer.Run(consumerCtx) }()
		DeferCleanup(func() {
			cancelConsumer()
			select {
			case runErr := <-consumerDone:
				Expect(runErr).NotTo(HaveOccurred())
			case <-time.After(5 * time.Second):
				Fail("metering consumer did not stop after its context was cancelled")
			}
		})

		select {
		case request := <-watchServer.requests:
			Expect(request.GetFilter()).To(Equal(watch.BuildFilter()))
		case <-ctx.Done():
			Fail("consumer did not establish its Watch stream before timeout")
		}

		var message *sarama.ConsumerMessage
		select {
		case message = <-partition.Messages():
			Expect(message).NotTo(BeNil())
		case consumerErr := <-partition.Errors():
			Fail("reading Kafka message: " + consumerErr.Error())
		case <-ctx.Done():
			Fail("timed out waiting for the lifecycle CloudEvent in Kafka")
		}

		Expect(message.Topic).To(Equal(topic))
		Expect(string(message.Key)).To(Equal(resourceID))

		var cloudEvent cloudevents.Event
		Expect(json.Unmarshal(message.Value, &cloudEvent)).To(Succeed())
		Expect(cloudEvent.ID()).To(Equal(eventID))
		Expect(cloudEvent.Type()).To(Equal(events.EventCreated))
		Expect(cloudEvent.SpecVersion()).To(Equal("1.0"))
		Expect(cloudEvent.Source()).To(Equal("osac-metering"))
		Expect(cloudEvent.Time().Equal(transitionTime)).To(BeTrue())
		Expect(cloudEvent.Extensions()).To(HaveKeyWithValue(schema.ExtResourceID, resourceID))
		Expect(cloudEvent.Extensions()).To(HaveKeyWithValue(schema.ExtResourceType, schema.ResourceTypeComputeInstance))
		Expect(cloudEvent.Extensions()).To(HaveKeyWithValue(schema.ExtTenant, "tenant-integration"))
		Expect(cloudEvent.Extensions()).To(HaveKeyWithValue(schema.ExtProject, "project-integration"))
		Expect(cloudEvent.DataContentType()).To(Equal(cloudevents.ApplicationJSON))

		var lifecycleData schema.LifecycleData
		Expect(cloudEvent.DataAs(&lifecycleData)).To(Succeed())
		Expect(lifecycleData.ResourceID).To(Equal(resourceID))
		Expect(lifecycleData.ResourceType).To(Equal(schema.ResourceTypeComputeInstance))
		Expect(lifecycleData.TenantID).To(Equal("tenant-integration"))
		Expect(lifecycleData.ProjectID).NotTo(BeNil())
		Expect(*lifecycleData.ProjectID).To(Equal("project-integration"))
		Expect(lifecycleData.CurrentState).To(Equal(events.ComputeInstanceStateRunning))
		Expect(lifecycleData.TransitionTime).To(Equal(transitionTime.Format(time.RFC3339Nano)))

		headers := make(map[string]string, len(message.Headers))
		for _, header := range message.Headers {
			headers[string(header.Key)] = string(header.Value)
		}
		Expect(headers).To(HaveKeyWithValue("ce_specversion", cloudEvent.SpecVersion()))
		Expect(headers).To(HaveKeyWithValue("ce_type", cloudEvent.Type()))
		Expect(headers).To(HaveKeyWithValue("ce_source", cloudEvent.Source()))
		Expect(headers).To(HaveKeyWithValue("ce_id", cloudEvent.ID()))
		Consistently(partition.Messages(), 250*time.Millisecond).ShouldNot(Receive())

		Eventually(func(g Gomega) {
			state, getErr := store.Get(ctx, resourceID)
			g.Expect(getErr).NotTo(HaveOccurred())
			g.Expect(state).NotTo(BeNil())
			g.Expect(state.FulfillmentVersion).To(Equal(int32(1)))
			g.Expect(state.IsBillable).To(BeTrue())
		}, 5*time.Second, 50*time.Millisecond).Should(Succeed())

		Expect(watchServer.callCount.Load()).To(Equal(int32(1)))
	})
})
