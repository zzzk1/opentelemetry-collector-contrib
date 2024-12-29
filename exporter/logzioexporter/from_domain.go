// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package logzioexporter

import (
	"strings"

	"github.com/jaegertracing/jaeger/model"
)

// ReferenceType is the reference type of one span to another
type ReferenceType string

// TraceID is the shared trace ID of all spans in the trace.
type TraceID string

// SpanID is the id of a span
type SpanID string

// ValueType is the type of a value stored in KeyValue struct.
type ValueType string

const (
	// ChildOf means a span is the child of another span
	ChildOf ReferenceType = "CHILD_OF"
	// FollowsFrom means a span follows from another span
	FollowsFrom ReferenceType = "FOLLOWS_FROM"

	// StringType indicates a string value stored in KeyValue
	StringType ValueType = "string"
	// BoolType indicates a Boolean value stored in KeyValue
	BoolType ValueType = "bool"
	// Int64Type indicates a 64bit signed integer value stored in KeyValue
	Int64Type ValueType = "int64"
	// Float64Type indicates a 64bit float value stored in KeyValue
	Float64Type ValueType = "float64"
	// BinaryType indicates an arbitrary byte array stored in KeyValue
	BinaryType ValueType = "binary"
)

// Reference is a reference from one span to another
type Reference struct {
	RefType ReferenceType `json:"refType"`
	TraceID TraceID       `json:"traceID"`
	SpanID  SpanID        `json:"spanID"`
}

// Process is the process emitting a set of spans
type Process struct {
	ServiceName string     `json:"serviceName"`
	Tags        []KeyValue `json:"tags"`
	// Alternative representation of tags for better kibana support
	Tag map[string]any `json:"tag,omitempty"`
}

// Log is a log emitted in a span
type Log struct {
	Timestamp uint64     `json:"timestamp"`
	Fields    []KeyValue `json:"fields"`
}

// KeyValue is a key-value pair with typed value.
type KeyValue struct {
	Key   string    `json:"key"`
	Type  ValueType `json:"type,omitempty"`
	Value any       `json:"value"`
}

// Service is the JSON struct for service:operation documents in ElasticSearch
type Service struct {
	ServiceName   string `json:"serviceName"`
	OperationName string `json:"operationName"`
}

// only for testing Span is ES database representation of the domain span.
type Span struct {
	TraceID       TraceID     `json:"traceID"`
	SpanID        SpanID      `json:"spanID"`
	ParentSpanID  SpanID      `json:"parentSpanID,omitempty"` // deprecated
	Flags         uint32      `json:"flags,omitempty"`
	OperationName string      `json:"operationName"`
	References    []Reference `json:"references"`
	StartTime     uint64      `json:"startTime"` // microseconds since Unix epoch
	// ElasticSearch does not support a UNIX Epoch timestamp in microseconds,
	// so Jaeger maps StartTime to a 'long' type. This extra StartTimeMillis field
	// works around this issue, enabling timerange queries.
	StartTimeMillis uint64     `json:"startTimeMillis"`
	Duration        uint64     `json:"duration"` // microseconds
	Tags            []KeyValue `json:"tags"`
	// Alternative representation of tags for better kibana support
	Tag     map[string]any `json:"tag,omitempty"`
	Logs    []Log          `json:"logs"`
	Process Process        `json:"process,omitempty"`
}

// newFromDomain creates fromDomain used to convert model span to db span
func newFromDomain(allTagsAsObject bool, tagKeysAsFields []string, tagDotReplacement string) fromDomain {
	tags := map[string]bool{}
	for _, k := range tagKeysAsFields {
		tags[k] = true
	}
	return fromDomain{allTagsAsFields: allTagsAsObject, tagKeysAsFields: tags, tagDotReplacement: tagDotReplacement}
}

// fromDomain is used to convert model span to db span
type fromDomain struct {
	allTagsAsFields   bool
	tagKeysAsFields   map[string]bool
	tagDotReplacement string
}

// fromDomainEmbedProcess converts model.Span into json.Span format.
// This format includes a ParentSpanID and an embedded Process.
func (fd fromDomain) fromDomainEmbedProcess(span *model.Span) *logzioSpan {
	return fd.convertSpanEmbedProcess(span)
}

func (fd fromDomain) convertSpanInternal(span *model.Span) logzioSpan {
	tags, tagsMap := fd.convertKeyValuesString(span.Tags)
	return logzioSpan{
		TraceID:         TraceID(span.TraceID.String()),
		SpanID:          SpanID(span.SpanID.String()),
		Flags:           uint32(span.Flags),
		OperationName:   span.OperationName,
		StartTime:       model.TimeAsEpochMicroseconds(span.StartTime),
		StartTimeMillis: model.TimeAsEpochMicroseconds(span.StartTime) / 1000,
		Duration:        model.DurationAsMicroseconds(span.Duration),
		Tags:            tags,
		Tag:             tagsMap,
		Logs:            fd.convertLogs(span.Logs),
	}
}

func (fd fromDomain) convertSpanEmbedProcess(span *model.Span) *logzioSpan {
	s := fd.convertSpanInternal(span)
	s.Process = fd.convertProcess(span.Process)
	s.References = fd.convertReferences(span)
	return &s
}

func (fd fromDomain) convertReferences(span *model.Span) []Reference {
	out := make([]Reference, 0, len(span.References))
	for _, ref := range span.References {
		out = append(out, Reference{
			RefType: fd.convertRefType(ref.RefType),
			TraceID: TraceID(ref.TraceID.String()),
			SpanID:  SpanID(ref.SpanID.String()),
		})
	}
	return out
}

func (fromDomain) convertRefType(refType model.SpanRefType) ReferenceType {
	if refType == model.FollowsFrom {
		return FollowsFrom
	}
	return ChildOf
}

func (fd fromDomain) convertKeyValuesString(keyValues model.KeyValues) ([]KeyValue, map[string]any) {
	var tagsMap map[string]any
	var kvs []KeyValue
	for _, kv := range keyValues {
		if kv.GetVType() != model.BinaryType && (fd.allTagsAsFields || fd.tagKeysAsFields[kv.Key]) {
			if tagsMap == nil {
				tagsMap = map[string]any{}
			}
			tagsMap[strings.ReplaceAll(kv.Key, ".", fd.tagDotReplacement)] = kv.Value()
		} else {
			kvs = append(kvs, convertKeyValue(kv))
		}
	}
	if kvs == nil {
		kvs = make([]KeyValue, 0)
	}
	return kvs, tagsMap
}

func (fromDomain) convertLogs(logs []model.Log) []Log {
	out := make([]Log, len(logs))
	for i, log := range logs {
		var kvs []KeyValue
		for _, kv := range log.Fields {
			kvs = append(kvs, convertKeyValue(kv))
		}
		out[i] = Log{
			Timestamp: model.TimeAsEpochMicroseconds(log.Timestamp),
			Fields:    kvs,
		}
	}
	return out
}

func (fd fromDomain) convertProcess(process *model.Process) Process {
	tags, tagsMap := fd.convertKeyValuesString(process.Tags)
	return Process{
		ServiceName: process.ServiceName,
		Tags:        tags,
		Tag:         tagsMap,
	}
}

func convertKeyValue(kv model.KeyValue) KeyValue {
	return KeyValue{
		Key:   kv.Key,
		Type:  ValueType(strings.ToLower(kv.VType.String())),
		Value: kv.AsString(),
	}
}
