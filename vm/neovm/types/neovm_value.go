package types

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"sort"

	"github.com/ontio/ontology/common"
	"github.com/ontio/ontology/vm/neovm/constants"
	"github.com/ontio/ontology/vm/neovm/errors"
)

type NeoVmValueType uint8

const (
	boolType NeoVmValueType = iota
	integerType
	bigintType
	bytearrayType
	interopType
	arrayType
	structType
	mapType
)

type VmValue struct {
	valType   NeoVmValueType
	integer   int64
	bigInt    *big.Int
	byteArray []byte
	structval StructValue
	array     *ArrayValue
	mapval    *MapValue
	interop   InteropValue
}

func VmValueFromInt64(val int64) VmValue {
	return VmValue{valType: integerType, integer: val}
}

func VmValueFromBytes(val []byte) (result VmValue, err error) {
	if len(val) > constants.MAX_BYTEARRAY_SIZE {
		err = errors.ERR_OVER_MAX_ITEM_SIZE
		return
	}
	result.valType = bytearrayType
	result.byteArray = val
	return
}

func VmValueFromBool(val bool) VmValue {
	if val {
		return VmValue{valType: boolType, integer: 1}
	} else {
		return VmValue{valType: boolType, integer: 0}
	}
}

func VmValueFromUint64(val uint64) VmValue {
	if val <= math.MaxInt64 {
		return VmValueFromInt64(int64(val))
	}

	b := big.NewInt(0)
	b.SetUint64(val)
	return VmValue{valType: bigintType, bigInt: b}
}

func VmValueFromBigInt(val *big.Int) (result VmValue, err error) {
	value, e := IntValFromBigInt(val)
	if e != nil {
		err = e
		return
	}

	return VmValueFromIntValue(value), nil
}

func VmValueFromArrayVal(array *ArrayValue) VmValue {
	return VmValue{valType: arrayType, array: array}
}

func VmValueFromStructVal(val StructValue) VmValue {
	return VmValue{valType: structType, structval: val}
}

func VmValueFromInteropValue(val InteropValue) VmValue {
	return VmValue{valType: interopType, interop: val}
}
func VmValueFromMapValue(val *MapValue) VmValue {
	return VmValue{valType: mapType, mapval: val}
}

func NewMapVmValue() VmValue {
	return VmValue{valType: mapType, mapval: NewMapValue()}
}

func VmValueFromIntValue(val IntValue) VmValue {
	if val.isbig {
		return VmValue{valType: bigintType, bigInt: val.bigint}
	} else {
		return VmValue{valType: integerType, integer: val.integer}
	}
}

func (self *VmValue) AsBytes() ([]byte, error) {
	switch self.valType {
	case integerType, boolType:
		return common.BigIntToNeoBytes(big.NewInt(self.integer)), nil
	case bigintType:
		return common.BigIntToNeoBytes(self.bigInt), nil
	case bytearrayType:
		return self.byteArray, nil
	case arrayType, mapType, structType, interopType:
		return nil, errors.ERR_BAD_TYPE
	default:
		panic("unreacheable!")
	}
}

func (self *VmValue) BuildParamToNative(sink *common.ZeroCopySink) error {
	switch self.valType {
	case bytearrayType:
		sink.WriteVarBytes(self.byteArray)
	case boolType:
		bool, err := self.AsBool()
		if err != nil {
			return err
		}
		sink.WriteBool(bool)
	case integerType:
		bs, err := self.AsBytes()
		if err != nil {
			return err
		}
		sink.WriteVarBytes(bs)
	case arrayType:
		sink.WriteVarBytes(BigIntToBytes(big.NewInt(int64(len(self.array.Data)))))
		for _, v := range self.array.Data {
			err := v.BuildParamToNative(sink)
			if err != nil {
				return err
			}
		}
	case structType:
		for _, v := range self.structval.Data {
			err := v.BuildParamToNative(sink)
			if err != nil {
				return err
			}
		}
	default:
		panic("unreacheable!")
	}
	return nil
}

func (self *VmValue) ToHexString() (interface{}, error) {
	switch self.valType {
	case boolType:
		boo, err := self.AsBool()
		if err != nil {
			return nil, err
		}
		if boo {
			return common.ToHexString([]byte{1}), nil
		} else {
			return common.ToHexString([]byte{0}), nil
		}
	case bytearrayType:
		return common.ToHexString(self.byteArray), nil
	case integerType:
		return common.ToHexString(common.BigIntToNeoBytes(big.NewInt(self.integer))), nil
	case bigintType:
		return common.ToHexString(common.BigIntToNeoBytes(self.bigInt)), nil
	case structType:
		var sstr []interface{}
		for i := 0; i < len(self.structval.Data); i++ {
			t, err := self.structval.Data[i].ToHexString()
			if err != nil {
				return nil, err
			}
			sstr = append(sstr, t)
		}
		return sstr, nil
	case arrayType:
		var sstr []interface{}
		for i := 0; i < len(self.array.Data); i++ {
			t, err := self.array.Data[i].ToHexString()
			if err != nil {
				return nil, err
			}
			sstr = append(sstr, t)
		}
		return sstr, nil
	case interopType:
		return common.ToHexString(self.interop.Data.ToArray()), nil
	default:
		return nil, fmt.Errorf("Unsupport type")
	}
}

func (self *VmValue) Deserialize(source *common.ZeroCopySource) error {
	t, eof := source.NextByte()
	if eof {
		return io.ErrUnexpectedEOF
	}
	switch t {
	case BooleanType:
		b, irregular, eof := source.NextBool()
		if eof {
			return io.ErrUnexpectedEOF
		}
		if irregular {
			return common.ErrIrregularData
		}
		*self = VmValueFromBool(b)
	case ByteArrayType:
		data, _, irregular, eof := source.NextVarBytes()
		if eof {
			return io.ErrUnexpectedEOF
		}
		if irregular {
			return common.ErrIrregularData
		}
		value, err := VmValueFromBytes(data)
		if err != nil {
			return err
		}
		*self = value
	case IntegerType:
		data, _, irregular, eof := source.NextVarBytes()
		if eof {
			return io.ErrUnexpectedEOF
		}
		if irregular {
			return common.ErrIrregularData
		}
		value, err := VmValueFromBigInt(common.BigIntFromNeoBytes(data))
		if err != nil {
			return err
		}
		*self = value
	case ArrayType:
		l, _, irregular, eof := source.NextVarUint()
		if eof {
			return io.ErrUnexpectedEOF
		}
		if irregular {
			return common.ErrIrregularData
		}
		arr := new(ArrayValue)
		for i := 0; i < int(l); i++ {
			v := VmValue{}
			err := v.Deserialize(source)
			if err != nil {
				return err
			}
			arr.Append(v)
		}
		*self = VmValueFromArrayVal(arr)
	case MapType:
		l, _, irregular, eof := source.NextVarUint()
		if eof {
			return io.ErrUnexpectedEOF
		}
		if irregular {
			return common.ErrIrregularData
		}
		mapValue := NewMapValue()
		for i := 0; i < int(l); i++ {
			keyValue := &VmValue{}
			err := keyValue.Deserialize(source)
			if err != nil {
				return err
			}
			v := &VmValue{}
			err = v.Deserialize(source)
			if err != nil {
				return err
			}
			mapValue.Set(*keyValue, *v)
		}
		*self = VmValueFromMapValue(mapValue)
	case StructType:
		l, _, irregular, eof := source.NextVarUint()
		if eof {
			return io.ErrUnexpectedEOF
		}
		if irregular {
			return common.ErrIrregularData
		}
		structValue := NewStructValue()
		for i := 0; i < int(l); i++ {
			v := VmValue{}
			err := v.Deserialize(source)
			if err != nil {
				return err
			}
			structValue.Append(v)
		}
		*self = VmValueFromStructVal(structValue)
	default:
		return fmt.Errorf("Unsupport type")

	}
	return nil
}

func (self *VmValue) Serialize(sink *common.ZeroCopySink) error {
	switch self.valType {
	case boolType:
		sink.WriteByte(BooleanType)
		boo, err := self.AsBool()
		if err != nil {
			return err
		}
		sink.WriteBool(boo)
	case bytearrayType:
		sink.WriteByte(ByteArrayType)
		sink.WriteVarBytes(self.byteArray)
	case bigintType:
		sink.WriteByte(byte(IntegerType))
		sink.WriteVarBytes(common.BigIntToNeoBytes(self.bigInt))
	case integerType:
		sink.WriteByte(byte(IntegerType))
		t := big.NewInt(self.integer)
		sink.WriteVarBytes(common.BigIntToNeoBytes(t))
	case arrayType:
		sink.WriteByte(ArrayType)
		sink.WriteVarUint(uint64(len(self.array.Data)))
		for i := 0; i < len(self.array.Data); i++ {
			err := self.array.Data[i].Serialize(sink)
			if err != nil {
				return err
			}
		}
	case mapType:
		sink.WriteByte(MapType)
		sink.WriteVarUint(uint64(len(self.mapval.Data)))
		var unsortKey []string
		for k := range self.mapval.Data {
			unsortKey = append(unsortKey, k)
		}
		sort.Strings(unsortKey)
		for _, key := range unsortKey {
			keyVal, err := VmValueFromBytes([]byte(key))
			if err != nil {
				return err
			}
			err = keyVal.Serialize(sink)
			if err != nil {
				return err
			}
			value := self.mapval.Data[key]
			err = value.Serialize(sink)
			if err != nil {
				return err
			}
		}
	case structType:
		sink.WriteByte(StructType)
		sink.WriteVarUint(uint64(len(self.structval.Data)))
		for _, item := range self.structval.Data {
			err := item.Serialize(sink)
			if err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("Unsupport type")
		//case interopType:
		//	intero, err := self.AsInteropValue()
		//	if err != nil {
		//		return err
		//	}
	}
	return nil
}

func (self *VmValue) AsInt64() (int64, error) {
	val, err := self.AsIntValue()
	if err != nil {
		return 0, err
	}
	if val.isbig {
		if val.bigint.IsInt64() == false {
			return 0, err
		}
		return val.bigint.Int64(), nil
	}

	return val.integer, nil
}

func (self *VmValue) AsIntValue() (IntValue, error) {
	switch self.valType {
	case integerType, boolType:
		return IntValFromInt(self.integer), nil
	case bigintType:
		return IntValFromBigInt(self.bigInt)
	case bytearrayType:
		return IntValFromNeoBytes(self.byteArray)
	case arrayType, mapType, structType, interopType:
		return IntValue{}, errors.ERR_BAD_TYPE
	default:
		panic("unreachable!")
	}
}

func (self *VmValue) AsBool() (bool, error) {
	switch self.valType {
	case integerType, boolType:
		return self.integer != 0, nil
	case bigintType:
		return self.bigInt.Sign() != 0, nil
	case bytearrayType:
		for _, b := range self.byteArray {
			if b != 0 {
				return true, nil
			}
		}
		return false, nil
	case structType, mapType:
		return true, nil
	case arrayType:
		return false, errors.ERR_BAD_TYPE
	case interopType:
		return self.interop != InteropValue{}, nil
	default:
		panic("unreachable!")
	}
}

func (self *VmValue) AsMapValue() (*MapValue, error) {
	switch self.valType {
	case mapType:
		return self.mapval, nil
	default:
		return nil, errors.ERR_BAD_TYPE
	}
}

func (self *VmValue) AsStructValue() (StructValue, error) {
	switch self.valType {
	case structType:
		return self.structval, nil
	default:
		return StructValue{}, errors.ERR_BAD_TYPE
	}
}

func (self *VmValue) AsArrayValue() (*ArrayValue, error) {
	switch self.valType {
	case arrayType:
		return self.array, nil
	default:
		return nil, errors.ERR_BAD_TYPE
	}
}

func (self *VmValue) AsInteropValue() (InteropValue, error) {
	switch self.valType {
	case interopType:
		return self.interop, nil
	default:
		return InteropValue{}, errors.ERR_BAD_TYPE
	}
}

func (self *VmValue) Equals(other VmValue) bool {
	v1, e1 := self.AsBytes()
	v2, e2 := other.AsBytes()
	if e1 == nil && e2 == nil { // both are primitive type
		return bytes.Equal(v1, v2)
	}

	// here more than one are compound type
	if self.valType != other.valType {
		return false
	}

	switch self.valType {
	case mapType:
		return self.mapval == other.mapval
	case structType:
		// todo: fix inconsistence
		return reflect.DeepEqual(self.structval, other.structval)
	case arrayType:
		return self.array == other.array
	case interopType:
		return self.interop.Equals(other.interop)
	default:
		panic("unreachable!")
	}
}

func (self *VmValue) GetMapKey() (string, error) {
	val, err := self.AsBytes()
	if err != nil {
		return "", err
	}
	return string(val), nil
}
