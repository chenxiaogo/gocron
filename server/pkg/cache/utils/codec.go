package utils

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"

	"github.com/vmihailenco/msgpack/v5"
)

// Codec is the codec interface.
type Codec interface {
	Encode(v interface{}) ([]byte, error)
	Decode(data []byte, v interface{}) error
}

// MsgPackCodec is the MsgPack codec.
type MsgPackCodec struct{}

func NewMsgPackCodec() *MsgPackCodec { return &MsgPackCodec{} }

func (c *MsgPackCodec) Encode(v interface{}) ([]byte, error) { return msgpack.Marshal(v) }
func (c *MsgPackCodec) Decode(data []byte, v interface{}) error { return msgpack.Unmarshal(data, v) }

// JSONCodec is the JSON codec.
type JSONCodec struct{}

func NewJSONCodec() *JSONCodec { return &JSONCodec{} }

func (c *JSONCodec) Encode(v interface{}) ([]byte, error) { return json.Marshal(v) }
func (c *JSONCodec) Decode(data []byte, v interface{}) error { return json.Unmarshal(data, v) }

// GZipCodec is a gzip-compressing codec wrapper.
type GZipCodec struct {
	codec Codec
}

func NewGZipCodec(codec Codec) *GZipCodec { return &GZipCodec{codec: codec} }

func (c *GZipCodec) Encode(v interface{}) ([]byte, error) {
	data, err := c.codec.Encode(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *GZipCodec) Decode(data []byte, v interface{}) error {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer r.Close()
	decompressed, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return c.codec.Decode(decompressed, v)
}

// CodecFactory creates codecs.
type CodecFactory struct{}

func (f *CodecFactory) GetCodec(codecType string) (Codec, error) {
	switch codecType {
	case "msgpack":
		return NewMsgPackCodec(), nil
	case "json":
		return NewJSONCodec(), nil
	case "gzip-msgpack":
		return NewGZipCodec(NewMsgPackCodec()), nil
	case "gzip-json":
		return NewGZipCodec(NewJSONCodec()), nil
	default:
		return nil, fmt.Errorf("unsupported codec type: %s", codecType)
	}
}