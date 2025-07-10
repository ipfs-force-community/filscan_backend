package jsonrpc

import (
	"encoding/json"
	"fmt"
	"reflect"
)

const eTempWSError = -1111111

type RPCConnectionError struct {
	err error
}

func (e *RPCConnectionError) Error() string {
	return e.err.Error()
}

func (e *RPCConnectionError) Unwrap() error {
	return e.err
}

type Errors struct {
	byType map[reflect.Type]ErrorCode
	byCode map[ErrorCode]reflect.Type
}

type ErrorCode = int

const FirstUserCode = 2

func NewErrors() Errors {
	return Errors{
		byType: map[reflect.Type]ErrorCode{},
		byCode: map[ErrorCode]reflect.Type{
			-1111111: reflect.TypeOf(&RPCConnectionError{}),
		},
	}
}

func (e *Errors) Register(c ErrorCode, typ interface{}) {
	rt := reflect.TypeOf(typ).Elem()
	if !rt.Implements(errorType) {
		panic("can't register non-error types")
	}
	
	e.byType[rt] = c
	e.byCode[c] = rt
}

type marshalable interface {
	json.Marshaler
	json.Unmarshaler
}

func NewError(code int, detail any, msg string, args ...any) *Error {
	return &Error{
		Code:    ErrorCode(code),
		Message: fmt.Sprintf(msg, args...),
		Detail:  detail,
	}
}

func NewMessage(msg string, args ...any) *Error {
	return &Error{
		Message: fmt.Sprintf(msg, args...),
	}
}

func NewCode(code int, msg string, args ...any) *Error {
	return &Error{
		Code:    ErrorCode(code),
		Message: fmt.Sprintf(msg, args...),
	}
}

func NewDetail(detail any, msg string, args ...any) *Error {
	return &Error{
		Message: fmt.Sprintf(msg, args...),
		Detail:  detail,
	}
}

var marshalableRT = reflect.TypeOf(new(marshalable)).Elem()

type Error struct {
	Code    ErrorCode       `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Detail  any             `json:"detail,omitempty"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

func (e *Error) Error() string {
	if e.Code >= -32768 && e.Code <= -32000 {
		return fmt.Sprintf("RPC error (%d): %s", e.Code, e.Message)
	}
	return e.Message
}

func (e *Error) val(errors *Errors) reflect.Value {
	if errors != nil {
		t, ok := errors.byCode[e.Code]
		if ok {
			var v reflect.Value
			if t.Kind() == reflect.Ptr {
				v = reflect.New(t.Elem())
			} else {
				v = reflect.New(t)
			}
			if len(e.Meta) > 0 && v.Type().Implements(marshalableRT) {
				_ = v.Interface().(marshalable).UnmarshalJSON(e.Meta)
			}
			if t.Kind() != reflect.Ptr {
				v = v.Elem()
			}
			return v
		}
	}
	
	return reflect.ValueOf(e)
}
