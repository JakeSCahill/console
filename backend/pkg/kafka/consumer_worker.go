// Copyright 2022 Redpanda Data, Inc.
//
// Use of this software is governed by the Business Source License
// included in the file https://github.com/redpanda-data/redpanda/blob/dev/licenses/bsl.md
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0

package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"

	"github.com/redpanda-data/console/backend/pkg/serde"
)

func (s *Service) startMessageWorker(ctx context.Context, wg *sync.WaitGroup,
	isMessageOK isMessageOkFunc, jobs <-chan *kgo.Record, resultsCh chan<- *TopicMessage,
	consumeReq TopicConsumeRequest,
) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("recovered from panic in message worker", zap.Any("error", r))
		}
	}()

	for record := range jobs {
		// We consume control records because the last message in a partition we expect might be a control record.
		// We need to acknowledge that we received the message but it is ineligible to be sent to the frontend.
		// Quit early if it is a control record!
		isControlRecord := record.Attrs.IsControl()
		if isControlRecord {
			topicMessage := &TopicMessage{
				PartitionID: record.Partition,
				Offset:      record.Offset,
				Timestamp:   record.Timestamp.UnixNano() / int64(time.Millisecond),
				IsMessageOk: false,
				MessageSize: int64(len(record.Key) + len(record.Value)),
			}

			select {
			case <-ctx.Done():
				return
			case resultsCh <- topicMessage:
				continue
			}
		}

		// Run Interpreter filter and check if message passes the filter
		deserializedRec := s.SerdeService.DeserializeRecord(
			ctx,
			record,
			serde.DeserializationOptions{
				MaxPayloadSize: s.Config.Console.MaxDeserializationPayloadSize,
				Troubleshoot:   consumeReq.Troubleshoot,
				IncludeRawData: consumeReq.IncludeRawPayload,
				KeyEncoding:    consumeReq.KeyDeserializer,
				ValueEncoding:  consumeReq.ValueDeserializer,
			})

		headersByKey := make(map[string][]byte, len(deserializedRec.Headers))
		headers := make([]MessageHeader, 0)
		for _, header := range deserializedRec.Headers {
			headersByKey[header.Key] = header.Value
			headers = append(headers, MessageHeader(header))
		}

		// Check if message passes filter code
		args := interpreterArguments{
			PartitionID:   record.Partition,
			Offset:        record.Offset,
			Timestamp:     record.Timestamp,
			Key:           deserializedRec.Key.DeserializedPayload,
			Value:         deserializedRec.Value.DeserializedPayload,
			HeadersByKey:  headersByKey,
			KeySchemaID:   deserializedRec.Key.SchemaID,
			ValueSchemaID: deserializedRec.Value.SchemaID,
		}

		isOK, outKey, outVal, err := isMessageOK(args)
		var errMessage string
		if err != nil {
			s.Logger.Debug("failed to check if message is ok", zap.Error(err))
			errMessage = fmt.Sprintf("Failed to check if message is ok (partition: '%v', offset: '%v'). Err: %v", record.Partition, record.Offset, err)
		}

		if isJSONable(deserializedRec.Value.Encoding) && outVal != nil {
			newJSON, err := json.Marshal(outVal)
			if err == nil {
				deserializedRec.Value.NormalizedPayload = newJSON
			}
		}

		if isJSONable(deserializedRec.Key.Encoding) && outKey != nil {
			newJSON, err := json.Marshal(outKey)
			if err == nil {
				deserializedRec.Key.NormalizedPayload = newJSON
			}
		}

		topicMessage := &TopicMessage{
			PartitionID:     record.Partition,
			Offset:          record.Offset,
			Timestamp:       record.Timestamp.UnixNano() / int64(time.Millisecond),
			Headers:         headers,
			Compression:     compressionTypeDisplayname(record.Attrs.CompressionType()),
			IsTransactional: record.Attrs.IsTransactional(),
			Key:             deserializedRec.Key,
			Value:           deserializedRec.Value,
			IsMessageOk:     isOK,
			ErrorMessage:    errMessage,
			MessageSize:     int64(len(record.Key) + len(record.Value)),
		}

		select {
		case <-ctx.Done():
			return
		case resultsCh <- topicMessage:
		}
	}
}

func isJSONable(enc serde.PayloadEncoding) bool {
	return enc == serde.PayloadEncodingJSON ||
		enc == serde.PayloadEncodingJSONSchema ||
		enc == serde.PayloadEncodingProtobuf ||
		enc == serde.PayloadEncodingProtobufSchema ||
		enc == serde.PayloadEncodingAvro
}
