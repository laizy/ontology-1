package neovm

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/ontio/ontology/vm/neovm/interfaces"
	"github.com/ontio/ontology/vm/neovm/types"
	"github.com/stretchr/testify/assert"
)

type Value interface{}

func value2json(t *testing.T, expect types.VmValue) []byte {
	e, err := expect.ConvertNeoVmValueHexString()
	assert.Nil(t, err)
	exp, err := json.Marshal(e)
	assert.Nil(t, err)

	return exp
}

func assertEqual(t *testing.T, expect, actual types.VmValue) {
	assert.Equal(t, value2json(t, expect), value2json(t, actual))
}

func newVmValue(t *testing.T, data Value) types.VmValue {
	switch v := data.(type) {
	case int8, int16, int32, int64, int, uint8, uint16, uint32, uint64, *big.Int, big.Int:
		val, err := types.VmValueFromBigInt(ToBigInt(v))
		assert.Nil(t, err)
		return val
	case bool:
		return types.VmValueFromBool(v)
	case []byte:
		val, err := types.VmValueFromBytes(v)
		assert.Nil(t, err)
		return val
	case string:
		val, err := types.VmValueFromBytes([]byte(v))
		assert.Nil(t, err)
		return val
	case []Value:
		arr := types.NewArrayValue()
		for _, item := range v {
			arr.Append(newVmValue(t, item))
		}

		return types.VmValueFromArrayVal(arr)
	case interfaces.Interop:
		return types.VmValueFromInteropValue(types.NewInteropValue(v))
	default:
		panic(fmt.Sprintf("newVmValue Invalid Type:%t", v))
	}
}

func checkStackOpCode(t *testing.T, code OpCode, origin, expected []Value) {
	executor := NewExecutor([]byte{byte(code)})
	for _, val := range origin {
		executor.EvalStack.Push(newVmValue(t, val))
	}
	err := executor.Execute()
	assert.Nil(t, err)
	stack := executor.EvalStack
	assert.Equal(t, len(expected), stack.Count())

	for i := 0; i < len(expected); i++ {
		val := expected[len(expected)-i-1]
		res, _ := stack.Pop()
		exp := newVmValue(t, val)
		assertEqual(t, res, exp)
	}
}

func TestDUPFROMALTSTACK(t *testing.T) {
	code := DUPFROMALTSTACK
	executor := NewExecutor([]byte{byte(code)})
	executor.AltStack.PushInt64(9999)
	executor.EvalStack.PushInt64(8888)
	executor.Execute()
	val,err := executor.EvalStack.Pop()
	assert.Equal(t, err, nil)
	i, err := val.AsInt64()
	assert.Equal(t, err, nil)
	assert.Equal(t, int64(9999),i)
}

func TestTOALTSTACK(t *testing.T){
	code := TOALTSTACK
	executor := NewExecutor([]byte{byte(code)})
	executor.EvalStack.PushInt64(8888)
	executor.Execute()
	val,err := executor.AltStack.Pop()
	i, err := val.AsInt64()
	assert.Equal(t, err, nil)
	assert.Equal(t, int64(8888),i)
}

func TestFROMALTSTACK(t *testing.T) {
	code := FROMALTSTACK
	executor := NewExecutor([]byte{byte(code)})
	executor.AltStack.PushInt64(8888)
	executor.Execute()
	val,err := executor.EvalStack.Pop()
	i, err := val.AsInt64()
	assert.Equal(t, err, nil)
	assert.Equal(t, int64(8888),i)
}


func TestLEFT(t *testing.T){
	code := LEFT
	executor := NewExecutor([]byte{byte(code)})
	executor.EvalStack.PushBytes([]byte("testhello"))
	executor.EvalStack.PushInt64(3)
	executor.Execute()

	val,err := executor.EvalStack.Pop()
	bs, err := val.AsBytes()
	assert.Equal(t, err, nil)
	bs2 := []byte("testhello")
	assert.Equal(t, bs, bs2[:3])
}
func TestRIGHT(t *testing.T){
	code := RIGHT
	executor := NewExecutor([]byte{byte(code)})
	executor.EvalStack.PushBytes([]byte("testhello"))
	executor.EvalStack.PushInt64(3)
	executor.Execute()

	val,err := executor.EvalStack.Pop()
	bs, err := val.AsBytes()
	assert.Equal(t, err, nil)
	bs2 := []byte("testhello")
	assert.Equal(t, bs, bs2[len(bs2)-3:])
}


func TestWITHIN(t *testing.T) {
	code := WITHIN
	executor := NewExecutor([]byte{byte(code)})
	executor.EvalStack.PushInt64(10000)
	executor.EvalStack.PushInt64(9999)
	executor.EvalStack.PushInt64(8888)
	executor.Execute()

	val,err := executor.EvalStack.Pop()
	i, err := val.AsInt64()
	assert.Equal(t, err, nil)
	assert.Equal(t, i, int64(0))
}
func TestStackOpCode(t *testing.T) {
	checkStackOpCode(t, SWAP, []Value{1, 2}, []Value{2, 1})
	checkStackOpCode(t, XDROP, []Value{3, 2, 1}, []Value{2})
	checkStackOpCode(t, XSWAP, []Value{3, 2, 1}, []Value{2, 3})
	checkStackOpCode(t, XTUCK, []Value{2, 1}, []Value{2, 2})
	checkStackOpCode(t, DEPTH, []Value{1, 2}, []Value{1, 2, 2})
	checkStackOpCode(t, DROP, []Value{1, 2}, []Value{1})
	checkStackOpCode(t, DUP, []Value{1, 2}, []Value{1, 2 ,2})
	checkStackOpCode(t, NIP, []Value{1, 2}, []Value{2})
	checkStackOpCode(t, OVER, []Value{1, 2}, []Value{1, 2, 1})
	checkStackOpCode(t, PICK, []Value{3, 2, 1}, []Value{3, 2, 3})
	checkStackOpCode(t, ROLL, []Value{3, 2, 1}, []Value{2, 3})
	checkStackOpCode(t, ROT, []Value{3, 1,1 , 1}, []Value{1, 1, 1,3})
	checkStackOpCode(t, TUCK, []Value{1, 2}, []Value{2, 1, 2})


	checkStackOpCode(t, INVERT, []Value{2}, []Value{-3})
	checkStackOpCode(t, AND, []Value{1, 2}, []Value{0})
	checkStackOpCode(t, OR, []Value{1, 2}, []Value{3})
	checkStackOpCode(t, XOR, []Value{1, 2}, []Value{3})
	checkStackOpCode(t, EQUAL, []Value{1, 2}, []Value{0})

	checkStackOpCode(t, INC, []Value{1}, []Value{2})
	checkStackOpCode(t, DEC, []Value{2}, []Value{1})
	checkStackOpCode(t, SIGN, []Value{1}, []Value{1})
	checkStackOpCode(t, NEGATE, []Value{1}, []Value{-1})
	checkStackOpCode(t, ABS, []Value{-9999}, []Value{9999})
	checkStackOpCode(t, NOT, []Value{1}, []Value{0})
	checkStackOpCode(t, NZ, []Value{1, 2}, []Value{1,1})
	checkStackOpCode(t, ADD, []Value{1, 2}, []Value{3})
	checkStackOpCode(t, SUB, []Value{1, 2}, []Value{-1})
	checkStackOpCode(t, MUL, []Value{1, 2}, []Value{2})
	checkStackOpCode(t, DIV, []Value{2, 1}, []Value{2})
	checkStackOpCode(t, MOD, []Value{1, 2}, []Value{1})
	//SHL未实现
	//checkStackOpCode(t, SHL, []int{1, 2}, []int{2})
	//checkStackOpCode(t, SHR, []int{1, 2}, []int{2, 1})
	checkStackOpCode(t, BOOLAND, []Value{1, 2}, []Value{1})
	checkStackOpCode(t, BOOLOR, []Value{1, 2}, []Value{1})
	checkStackOpCode(t, NUMEQUAL, []Value{1, 2}, []Value{0})
	checkStackOpCode(t, NUMNOTEQUAL, []Value{1, 2}, []Value{1})
	checkStackOpCode(t, LT, []Value{1, 2}, []Value{1})
	checkStackOpCode(t, GT, []Value{1, 2}, []Value{0})
	checkStackOpCode(t, LTE, []Value{1, 2}, []Value{1})
	checkStackOpCode(t, GTE, []Value{1, 2}, []Value{0})
	checkStackOpCode(t, MIN, []Value{1, 2}, []Value{1})
	checkStackOpCode(t, MAX, []Value{1, 2}, []Value{2})

}

func TestArithmetic(t *testing.T) {
	checkStackOpCode(t, ADD, []Value{1, 2}, []Value{3})
	checkStackOpCode(t, SUB, []Value{1, 2}, []Value{-1})

	checkStackOpCode(t, MUL, []Value{3, 2}, []Value{6})

	checkStackOpCode(t, DIV, []Value{3, 2}, []Value{1})
	checkStackOpCode(t, DIV, []Value{103, 2}, []Value{51})

	checkStackOpCode(t, MAX, []Value{3, 2}, []Value{3})
	checkStackOpCode(t, MAX, []Value{-3, 2}, []Value{2})

	checkStackOpCode(t, MIN, []Value{3, 2}, []Value{2})
	checkStackOpCode(t, MIN, []Value{-3, 2}, []Value{-3})

	checkStackOpCode(t, SIGN, []Value{3}, []Value{1})
	checkStackOpCode(t, SIGN, []Value{-3}, []Value{-1})
	checkStackOpCode(t, SIGN, []Value{0}, []Value{0})

	checkStackOpCode(t, INC, []Value{-10}, []Value{-9})
	checkStackOpCode(t, DEC, []Value{-10}, []Value{-11})
	checkStackOpCode(t, NEGATE, []Value{-10}, []Value{10})
	checkStackOpCode(t, ABS, []Value{-10}, []Value{10})

	checkStackOpCode(t, NOT, []Value{1}, []Value{0})
	checkStackOpCode(t, NOT, []Value{0}, []Value{1})

	checkStackOpCode(t, NZ, []Value{0}, []Value{0})
	checkStackOpCode(t, NZ, []Value{-10}, []Value{1})
	checkStackOpCode(t, NZ, []Value{10}, []Value{1})
}

func TestArrayOpCode(t *testing.T) {
	checkStackOpCode(t, ARRAYSIZE, []Value{"12345"}, []Value{5})
	checkStackOpCode(t, ARRAYSIZE, []Value{[]Value{1, 2, 3}}, []Value{3})
	checkStackOpCode(t, ARRAYSIZE, []Value{[]Value{}}, []Value{0})

	checkStackOpCode(t, PACK, []Value{"aaa", "bbb", "ccc", 3}, []Value{[]Value{"ccc", "bbb", "aaa"}})

	checkStackOpCode(t, UNPACK, []Value{[]Value{"ccc", "bbb", "aaa"}}, []Value{"aaa", "bbb", "ccc"})

	checkStackOpCode(t, PICKITEM, []Value{[]Value{"ccc", "bbb", "aaa"}, 0}, []Value{"ccc"})

	checkStackOpCode(t, SIZE, []Value{"12345"}, []Value{5})
	checkStackOpCode(t, CAT, []Value{"aaa", "bbb"}, []Value{"aaabbb"})
	checkStackOpCode(t, SUBSTR, []Value{"aaabbb", 1,3}, []Value{"aab"})
	checkStackOpCode(t, LEFT, []Value{"aaabbb", 3},[]Value{"aaa"})
	checkStackOpCode(t, RIGHT, []Value{"aaabbb", 3},[]Value{"bbb"})
}

func TestAssertEqual(t *testing.T) {
	val1 := newVmValue(t, -12345678910)
	buf, _ := val1.AsBytes()
	val2 := newVmValue(t, buf)

	assertEqual(t, val1, val2)
}
