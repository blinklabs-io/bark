// Copyright 2026 Blink Labs Software

package databasev1alpha1

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestSnapshotFormatContract(t *testing.T) {
	file := File_v1alpha1_database_database_proto
	if file == nil {
		t.Fatal("database descriptor is missing")
	}
	formatEnum := file.Enums().ByName("SnapshotFormat")
	if formatEnum == nil {
		t.Fatal("SnapshotFormat enum is missing")
	}
	values := map[protoreflect.Name]protoreflect.EnumNumber{
		"SNAPSHOT_FORMAT_UNSPECIFIED": 0,
		"SNAPSHOT_FORMAT_NATIVE":      1,
		"SNAPSHOT_FORMAT_SCLS":        2,
	}
	for name, number := range values {
		value := formatEnum.Values().ByName(name)
		if value == nil {
			t.Errorf("SnapshotFormat value %s is missing", name)
			continue
		}
		if value.Number() != number {
			t.Errorf(
				"SnapshotFormat value %s = %d, want %d",
				name,
				value.Number(),
				number,
			)
		}
	}

	assertFormatField(t, (&CreateSnapshotRequest{}).ProtoReflect(), 3)
	assertFormatField(t, (&SnapshotInfo{}).ProtoReflect(), 9)
	assertFormatField(t, (&GetSnapshotStatusResponse{}).ProtoReflect(), 3)
}

func TestSnapshotFormatWireRoundTrip(t *testing.T) {
	original := &CreateSnapshotRequest{Name: "daily", Description: "ledger"}
	formatField := original.ProtoReflect().
		Descriptor().
		Fields().
		ByName("format")
	if formatField == nil {
		t.Fatal("CreateSnapshotRequest.format is missing")
	}
	if got := original.ProtoReflect().Get(formatField).Enum(); got != 0 {
		t.Fatalf("omitted format = %d, want 0", got)
	}

	original.ProtoReflect().Set(
		formatField,
		protoreflect.ValueOfEnum(2),
	)
	wire, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var decoded CreateSnapshotRequest
	if err := proto.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	decodedFormat := decoded.ProtoReflect().
		Descriptor().
		Fields().
		ByName("format")
	if got := decoded.ProtoReflect().Get(decodedFormat).Enum(); got != 2 {
		t.Fatalf("round-trip format = %d, want 2", got)
	}
}

func assertFormatField(
	t *testing.T,
	message protoreflect.Message,
	number protoreflect.FieldNumber,
) {
	t.Helper()
	field := message.Descriptor().Fields().ByName("format")
	if field == nil {
		t.Errorf("%s.format is missing", message.Descriptor().Name())
		return
	}
	if field.Number() != number {
		t.Errorf(
			"%s.format field number = %d, want %d",
			message.Descriptor().Name(),
			field.Number(),
			number,
		)
	}
	if field.Enum() == nil || field.Enum().FullName() != formatEnumName {
		t.Errorf(
			"%s.format type = %v, want %s",
			message.Descriptor().Name(),
			field.Enum(),
			formatEnumName,
		)
	}
}

const formatEnumName = protoreflect.FullName(
	"bark.v1alpha1.database.SnapshotFormat",
)
