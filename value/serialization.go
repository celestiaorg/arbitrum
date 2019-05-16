package value

import (
	"errors"
	"github.com/offchainlabs/arb-avm/code"
	"math/big"
)

//go:generate protoc -I.. -I. --go_out=paths=source_relative:. value.proto

func NewBigIntBuf(buf *big.Int) *BigIntegerBuf {
	return &BigIntegerBuf{
		Value: buf.Bytes(),
	}
}

func NewBigIntFromBuf(buf *BigIntegerBuf) *big.Int {
	return new(big.Int).SetBytes(buf.Value)
}

func NewHashBuf(h [32]byte) *HashBuf {
	return &HashBuf{
		Value: h[:],
	}
}
func NewHashFromBuf(buf *HashBuf) [32]byte {
	var ret [32]byte
	copy(ret[:], buf.Value)
	return ret
}

func NewOperationBuf(op Operation) *OperationBuf {
	switch op := op.(type) {
	case BasicOperation:
		return &OperationBuf{
			OpCode:uint32(op.GetOp()),
		}
	case ImmediateOperation:
		return &OperationBuf{
			OpCode:uint32(op.GetOp()),
			Immediate: NewValueBuf(op.Val),
		}
	default:
		panic("unknown value typecode")
	}
}

func NewOperationFromBuf(buf *OperationBuf) (Operation, error) {
	if buf.Immediate != nil {
		val, err := NewValueFromBuf(buf.Immediate)
		return ImmediateOperation{
			code.Opcode(buf.OpCode),
			val,
		}, err
	} else {
		return BasicOperation{code.Opcode(buf.OpCode)}, nil
	}
}

func NewCodePointBuf(val CodePointValue) *CodePointBuf {
	return &CodePointBuf{
		Pc: val.InsnNum,
		Op: NewOperationBuf(val.Op),
		NextHash: NewHashBuf(val.NextHash),
	}
}

func NewCodePointFromBuf(buf *CodePointBuf) (CodePointValue, error) {
	op, err := NewOperationFromBuf(buf.Op)
	return CodePointValue{
		buf.Pc,
		op,
		NewHashFromBuf(buf.NextHash),
	}, err
}

func NewTupleBuf(val TupleValue) *TupleBuf {
	values := make([]*ValueBuf, 0, val.itemCount)
	for _, val := range val.Contents() {
		values = append(values, NewValueBuf(val))
	}
	return &TupleBuf{
		Values: values,
	}
}

func NewTupleFromBuf(buf *TupleBuf) (TupleValue, error) {
	values := make([]Value, 0, len(buf.Values))
	for _, val := range buf.Values {
		t, err := NewValueFromBuf(val)
		if err != nil {
			return TupleValue{}, err
		}
		values = append(values, t)
	}
	return NewTupleFromSlice(values)
}

func NewValueBuf(val Value) *ValueBuf {
	switch val := val.(type) {
	case IntValue:
		return &ValueBuf{
			Type: uint32(TypeCodeInt),
			Value: &ValueBuf_IntVal{NewBigIntBuf(val.val)},
		}
	case CodePointValue:
		return &ValueBuf{
			Type: TypeCodeCodePoint,
			Value: &ValueBuf_CodePointVal{NewCodePointBuf(val)},
		}
	case TupleValue:
		return &ValueBuf{
			Type: TypeCodeCodePoint,
			Value: &ValueBuf_TupleVal{NewTupleBuf(val)},
		}
	default:
		panic("unknown value typecode")
	}
}

func NewValueFromBuf(buf *ValueBuf) (Value, error) {

	switch val := buf.Value.(type) {
	case *ValueBuf_IntVal:
		return NewIntValue(NewBigIntFromBuf(val.IntVal)), nil
	case *ValueBuf_CodePointVal:
		return NewCodePointFromBuf(val.CodePointVal)
	case *ValueBuf_TupleVal:
		return NewTupleFromBuf(val.TupleVal)
	default:
		return nil, errors.New("unknown value typecode")
	}
}