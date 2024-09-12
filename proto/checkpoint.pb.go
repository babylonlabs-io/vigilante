// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.33.0
// 	protoc        v3.6.1
// source: checkpoint.proto

package proto

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// StoredTx stores a BTC transaction and its ID
type StoredTx struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	TxId []byte `protobuf:"bytes,1,opt,name=tx_id,json=txId,proto3" json:"tx_id,omitempty"` // chainhash.Hash serialized as bytes
	Tx   []byte `protobuf:"bytes,2,opt,name=tx,proto3" json:"tx,omitempty"`                 // wire.MsgTx serialized as bytes
}

func (x *StoredTx) Reset() {
	*x = StoredTx{}
	if protoimpl.UnsafeEnabled {
		mi := &file_checkpoint_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StoredTx) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StoredTx) ProtoMessage() {}

func (x *StoredTx) ProtoReflect() protoreflect.Message {
	mi := &file_checkpoint_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StoredTx.ProtoReflect.Descriptor instead.
func (*StoredTx) Descriptor() ([]byte, []int) {
	return file_checkpoint_proto_rawDescGZIP(), []int{0}
}

func (x *StoredTx) GetTxId() []byte {
	if x != nil {
		return x.TxId
	}
	return nil
}

func (x *StoredTx) GetTx() []byte {
	if x != nil {
		return x.Tx
	}
	return nil
}

// StoredCheckpoint holds two transactions and an epoch number
type StoredCheckpoint struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Tx1   *StoredTx `protobuf:"bytes,1,opt,name=tx1,proto3" json:"tx1,omitempty"`
	Tx2   *StoredTx `protobuf:"bytes,2,opt,name=tx2,proto3" json:"tx2,omitempty"`
	Epoch uint64    `protobuf:"varint,3,opt,name=epoch,proto3" json:"epoch,omitempty"`
}

func (x *StoredCheckpoint) Reset() {
	*x = StoredCheckpoint{}
	if protoimpl.UnsafeEnabled {
		mi := &file_checkpoint_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StoredCheckpoint) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StoredCheckpoint) ProtoMessage() {}

func (x *StoredCheckpoint) ProtoReflect() protoreflect.Message {
	mi := &file_checkpoint_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StoredCheckpoint.ProtoReflect.Descriptor instead.
func (*StoredCheckpoint) Descriptor() ([]byte, []int) {
	return file_checkpoint_proto_rawDescGZIP(), []int{1}
}

func (x *StoredCheckpoint) GetTx1() *StoredTx {
	if x != nil {
		return x.Tx1
	}
	return nil
}

func (x *StoredCheckpoint) GetTx2() *StoredTx {
	if x != nil {
		return x.Tx2
	}
	return nil
}

func (x *StoredCheckpoint) GetEpoch() uint64 {
	if x != nil {
		return x.Epoch
	}
	return 0
}

var File_checkpoint_proto protoreflect.FileDescriptor

var file_checkpoint_proto_rawDesc = []byte{
	0x0a, 0x10, 0x63, 0x68, 0x65, 0x63, 0x6b, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x12, 0x05, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x2f, 0x0a, 0x08, 0x53, 0x74, 0x6f,
	0x72, 0x65, 0x64, 0x54, 0x78, 0x12, 0x13, 0x0a, 0x05, 0x74, 0x78, 0x5f, 0x69, 0x64, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x74, 0x78, 0x49, 0x64, 0x12, 0x0e, 0x0a, 0x02, 0x74, 0x78,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x02, 0x74, 0x78, 0x22, 0x6e, 0x0a, 0x10, 0x53, 0x74,
	0x6f, 0x72, 0x65, 0x64, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x12, 0x21,
	0x0a, 0x03, 0x74, 0x78, 0x31, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x2e, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x64, 0x54, 0x78, 0x52, 0x03, 0x74, 0x78,
	0x31, 0x12, 0x21, 0x0a, 0x03, 0x74, 0x78, 0x32, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x64, 0x54, 0x78, 0x52,
	0x03, 0x74, 0x78, 0x32, 0x12, 0x14, 0x0a, 0x05, 0x65, 0x70, 0x6f, 0x63, 0x68, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x05, 0x65, 0x70, 0x6f, 0x63, 0x68, 0x42, 0x2b, 0x5a, 0x29, 0x67, 0x69,
	0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x62, 0x61, 0x62, 0x79, 0x6c, 0x6f, 0x6e,
	0x6c, 0x61, 0x62, 0x73, 0x2d, 0x69, 0x6f, 0x2f, 0x76, 0x69, 0x67, 0x69, 0x6c, 0x61, 0x6e, 0x74,
	0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_checkpoint_proto_rawDescOnce sync.Once
	file_checkpoint_proto_rawDescData = file_checkpoint_proto_rawDesc
)

func file_checkpoint_proto_rawDescGZIP() []byte {
	file_checkpoint_proto_rawDescOnce.Do(func() {
		file_checkpoint_proto_rawDescData = protoimpl.X.CompressGZIP(file_checkpoint_proto_rawDescData)
	})
	return file_checkpoint_proto_rawDescData
}

var file_checkpoint_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_checkpoint_proto_goTypes = []interface{}{
	(*StoredTx)(nil),         // 0: proto.StoredTx
	(*StoredCheckpoint)(nil), // 1: proto.StoredCheckpoint
}
var file_checkpoint_proto_depIdxs = []int32{
	0, // 0: proto.StoredCheckpoint.tx1:type_name -> proto.StoredTx
	0, // 1: proto.StoredCheckpoint.tx2:type_name -> proto.StoredTx
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_checkpoint_proto_init() }
func file_checkpoint_proto_init() {
	if File_checkpoint_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_checkpoint_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StoredTx); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_checkpoint_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StoredCheckpoint); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_checkpoint_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_checkpoint_proto_goTypes,
		DependencyIndexes: file_checkpoint_proto_depIdxs,
		MessageInfos:      file_checkpoint_proto_msgTypes,
	}.Build()
	File_checkpoint_proto = out.File
	file_checkpoint_proto_rawDesc = nil
	file_checkpoint_proto_goTypes = nil
	file_checkpoint_proto_depIdxs = nil
}
