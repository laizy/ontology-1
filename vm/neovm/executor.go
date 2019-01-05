/*
 * Copyright (C) 2018 The ontology Authors
 * This file is part of The ontology library.
 *
 * The ontology is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The ontology is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public License
 * along with The ontology.  If not, see <http://www.gnu.org/licenses/>.
 */

package neovm

import (
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"github.com/ontio/ontology-crypto/keypair"
	"github.com/ontio/ontology/core/signature"
	"github.com/ontio/ontology/vm/neovm/errors"
	"github.com/ontio/ontology/vm/neovm/types"
	"golang.org/x/crypto/ripemd160"
)

func NewExecutor(code []byte) *Executor {
	var engine Executor
	engine.EvalStack = NewValueStack(STACK_LIMIT)
	engine.AltStack = NewValueStack(STACK_LIMIT)
	context := NewExecutionContext(code)
	engine.Context = context
	engine.State = BREAK
	return &engine
}

type Executor struct {
	EvalStack *ValueStack
	AltStack  *ValueStack
	State     VMState
	Callers   []*ExecutionContext
	Context   *ExecutionContext
}

func (self *Executor) PopContext() (*ExecutionContext, error) {
	total := len(self.Callers)
	if total == 0 {
		return nil, errors.ERR_INDEX_OUT_OF_BOUND
	}
	context := self.Callers[total-1]
	self.Callers = self.Callers[:total-1]
	return context, nil
}

func (self *Executor) PushContext(context *ExecutionContext) {
	//todo : check limit
	self.Callers = append(self.Callers, context)
}

func (self *Executor) Execute() error {
	self.State = self.State & (^BREAK)
	for self.Context != nil {
		if self.State == FAULT || self.State == HALT || self.State == BREAK {
			break
		}
		opcode, eof := self.Context.ReadOpCode()
		if eof {
			break
		}
		var err error
		self.State, err = self.ExecuteOp(opcode, self.Context)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *Executor) ExecuteOp(opcode OpCode, context *ExecutionContext) (VMState, error) {
	if opcode >= PUSHBYTES1 && opcode <= PUSHBYTES75 {
		buf, err := context.OpReader.ReadBytes(int(opcode))
		if err != nil {
			return FAULT, err
		}
		val, err := types.VmValueFromBytes(buf)
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}
		return NONE, nil
	}

	switch opcode {
	case PUSH0:
		err := self.EvalStack.Push(types.VmValueFromInt64(0))
		if err != nil {
			return FAULT, err
		}
	case PUSHDATA1, PUSHDATA2, PUSHDATA4:
		var numBytes int
		if opcode == PUSHDATA1 {
			d, err := context.OpReader.ReadByte()
			if err != nil {
				return FAULT, err
			}

			numBytes = int(d)
		} else if opcode == PUSHDATA2 {
			num, err := context.OpReader.ReadUint16()
			if err != nil {
				return FAULT, err
			}
			numBytes = int(num)
		} else {
			num, err := context.OpReader.ReadUint32()
			if err != nil {
				return FAULT, err
			}
			numBytes = int(num)
		}

		data, err := context.OpReader.ReadBytes(numBytes)
		val, err := types.VmValueFromBytes(data)
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}
	case PUSHM1, PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8, PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16:
		val := int64(opcode) - int64(PUSH1) + 1
		err := self.EvalStack.Push(types.VmValueFromInt64(val))
		if err != nil {
			return FAULT, err
		}
		// Flow control
	case NOP:
		return NONE, nil
	case JMP, JMPIF, JMPIFNOT, CALL:
		if opcode == CALL {
			caller := context.Clone()
			caller.SetInstructionPointer(int64(caller.GetInstructionPointer() + 2))
			self.PushContext(caller)
			opcode = JMP
		}

		num, err := context.OpReader.ReadInt16()
		if err != nil {
			return FAULT, err
		}
		offset := int(num)
		offset = context.GetInstructionPointer() + offset - 3

		if offset < 0 || offset > len(context.Code) {
			return FAULT, errors.ERR_FAULT
		}
		var needJmp = true
		if opcode != JMP {
			val, err := self.EvalStack.PopAsBool()
			if err != nil {
				return FAULT, err
			}
			if opcode == JMPIF {
				needJmp = val
			} else {
				needJmp = !val
			}
		}

		if needJmp {
			context.SetInstructionPointer(int64(offset))
		}
	case RET:
		self.Context, _ = self.PopContext()

		/*
			//todo
				APPCALL  OpCode = 0x67
				SYSCALL  OpCode = 0x68
		*/
		// Stack

	case DUPFROMALTSTACK:
		val, err := self.AltStack.Peek(0)
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}
	case TOALTSTACK:
		val, err := self.EvalStack.Pop()
		if err != nil {
			return FAULT, err
		}
		err = self.AltStack.Push(val)
		if err != nil {
			return FAULT, err
		}
	case FROMALTSTACK:
		val, err := self.AltStack.Pop()
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}

	case XDROP: // XDROP is zero based
		n, err := self.EvalStack.PopAsInt64()
		if err != nil {
			return FAULT, err
		}
		_, err = self.EvalStack.Remove(n)
		if err != nil {
			return FAULT, err
		}
	case XSWAP:
		n, err := self.EvalStack.PopAsInt64()
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.Swap(0, n)
		if err != nil {
			return FAULT, err
		}
	case XTUCK:
		n, err := self.EvalStack.PopAsInt64()
		if err != nil {
			return FAULT, err
		}

		val, err := self.EvalStack.Peek(0)
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.Insert(n, val)
		if err != nil {
			return FAULT, err
		}
	case DEPTH:
		err := self.EvalStack.PushInt64(int64(self.EvalStack.Count()))
		if err != nil {
			return FAULT, err
		}
	case DROP:
		_, err := self.EvalStack.Pop()
		if err != nil {
			return FAULT, err
		}
	case DUP:
		val, err := self.EvalStack.Peek(0)
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}
	case NIP:
		_, val, err := self.EvalStack.PopPair()
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}
	case OVER:
		val, err := self.EvalStack.Peek(1)
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}
	case PICK:
		n, err := self.EvalStack.PopAsInt64()
		if err != nil {
			return FAULT, err
		}

		val, err := self.EvalStack.Peek(n)
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}
	case ROLL, ROT:
		var n int64
		var err error
		if opcode == ROT {
			n = 2
		} else {
			n, err = self.EvalStack.PopAsInt64()
			if err != nil {
				return FAULT, err
			}
		}

		// todo: clearly define the behave when n ==0 and stack is empty
		val, err := self.EvalStack.Remove(n)
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}
	case SWAP: // The top two items on the stack are swapped.
		err := self.EvalStack.Swap(0, 1)
		if err != nil {
			return FAULT, err
		}
	case TUCK: // The item at the top of the stack is copied and inserted before the second-to-top item.
		x1, x2, err := self.EvalStack.PopPair()
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.PushMany(x2, x1, x2)
		if err != nil {
			return FAULT, err
		}
		// Splice
	case CAT:
		left, right, err := self.EvalStack.PopPairAsBytes()
		if err != nil {
			return FAULT, err
		}

		val := make([]byte, 0, len(left)+len(right))
		val = append(val, left...)
		val = append(val, right...)
		err = self.EvalStack.PushBytes(val)
		if err != nil {
			return FAULT, err
		}
	case SUBSTR:
		start, count, err := self.EvalStack.PopPairAsInt64()
		if err != nil {
			return FAULT, err
		}
		arr, err := self.EvalStack.PopAsBytes()
		if err != nil {
			return FAULT, err
		}

		length := int64(len(arr))
		if start < 0 || start > length {
			return FAULT, errors.ERR_OVER_MAX_ARRAY_SIZE
		}
		if count < 0 || count > length {
			return FAULT, errors.ERR_OVER_MAX_ARRAY_SIZE
		}
		end := start + count
		if end > length {
			return FAULT, errors.ERR_OVER_MAX_ARRAY_SIZE
		}

		b := arr[start:end]
		err = self.EvalStack.PushBytes(b)
		if err != nil {
			return FAULT, err
		}

	case LEFT:
		count, err := self.EvalStack.PopAsInt64()
		if err != nil {
			return FAULT, err
		}
		arr, err := self.EvalStack.PopAsBytes()
		if err != nil {
			return FAULT, err
		}

		length := int64(len(arr))
		if count < 0 || count > length {
			return FAULT, errors.ERR_OVER_MAX_ARRAY_SIZE
		}

		b := arr[:count]
		err = self.EvalStack.PushBytes(b)
		if err != nil {
			return FAULT, err
		}
	case RIGHT:
		count, err := self.EvalStack.PopAsInt64()
		if err != nil {
			return FAULT, err
		}
		arr, err := self.EvalStack.PopAsBytes()
		if err != nil {
			return FAULT, err
		}

		length := int64(len(arr))
		if count < 0 || count > length {
			return FAULT, errors.ERR_OVER_MAX_ARRAY_SIZE
		}

		b := arr[length-count:]
		err = self.EvalStack.PushBytes(b)
		if err != nil {
			return FAULT, err
		}
	case SIZE:
		arr, err := self.EvalStack.PopAsBytes()
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.PushInt64(int64(len(arr)))
		if err != nil {
			return FAULT, err
		}
	// Bitwise logic
	case INVERT:
		left, err := self.EvalStack.PopAsIntValue()
		if err != nil {
			return FAULT, err
		}
		val := left.Not()
		err = self.EvalStack.Push(types.VmValueFromIntValue(val))
		if err != nil {
			return FAULT, err
		}
	case AND, OR, XOR:
		left, right, err := self.EvalStack.PopPairAsIntVal()
		if err != nil {
			return FAULT, err
		}

		var val types.IntValue
		switch opcode {
		case AND:
			val, err = left.And(right)
		case OR:
			val, err = left.Or(right)
		case XOR:
			val, err = left.Xor(right)
		default:
			panic("unreachable")
		}
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.Push(types.VmValueFromIntValue(val))
		if err != nil {
			return FAULT, err
		}
	case EQUAL:
		left, right, err := self.EvalStack.PopPair()
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.PushBool(left.Equals(right))
		if err != nil {
			return FAULT, err
		}
	case INC, DEC, SIGN, NEGATE, ABS, NZ:
		x, err := self.EvalStack.PopAsIntValue()
		if err != nil {
			return FAULT, err
		}

		var val types.IntValue
		switch opcode {
		case INC:
			val, err = x.Add(types.IntValFromInt(1))
		case DEC:
			val, err = x.Sub(types.IntValFromInt(1))
		case SIGN:
			cmp := x.Cmp(types.IntValFromInt(0))
			val = types.IntValFromInt(int64(cmp))
		case NEGATE:
			val, err = types.IntValFromInt(0).Sub(x)
		case ABS:
			val = x.Abs()
		case NZ:
			cmp := x.Cmp(types.IntValFromInt(0))
			if cmp == 0 {
				val = types.IntValFromInt(0)
			} else {
				val = types.IntValFromInt(1)
			}
		default:
			panic("unreachable")
		}
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.Push(types.VmValueFromIntValue(val))
		if err != nil {
			return FAULT, err
		}
	case ADD, SUB, MUL, DIV, MOD, MAX, MIN:
		left, right, err := self.EvalStack.PopPairAsIntVal()
		if err != nil {
			return FAULT, err
		}
		var val types.IntValue
		switch opcode {
		case ADD:
			val, err = left.Add(right)
		case SUB:
			val, err = left.Sub(right)
		case MUL:
			val, err = left.Mul(right)
		case DIV:
			val, err = left.Div(right)
		case MOD:
			val, err = left.Mod(right)
		case MAX:
			val, err = left.Max(right)
		case MIN:
			val, err = left.Min(right)
		default:
			panic("unreachable")
		}
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.Push(types.VmValueFromIntValue(val))
		if err != nil {
			return FAULT, err
		}
	case SHL, SHR:
		x2, err := self.EvalStack.PopAsIntValue()
		if err != nil {
			return FAULT, err
		}
		x1, err := self.EvalStack.PopAsIntValue()
		if err != nil {
			return FAULT, err
		}
		var res types.IntValue
		switch opcode {
		case SHL:
			res, err = x1.Lsh(x2)
			if err != nil {
				return FAULT, err
			}
		case SHR:
			res, err = x1.Rsh(x2)
			if err != nil {
				return FAULT, err
			}
		default:
			panic("unreachable")
		}
		b := types.VmValueFromIntValue(res)
		err = self.EvalStack.Push(b)
		if err != nil {
			return FAULT, err
		}
	case NUMEQUAL, NUMNOTEQUAL, LT, GT, LTE, GTE:
		left, right, err := self.EvalStack.PopPairAsIntVal()
		if err != nil {
			return FAULT, err
		}
		var val bool
		switch opcode {
		case NUMEQUAL:
			val = left.Cmp(right) == 0
		case NUMNOTEQUAL:
			val = left.Cmp(right) != 0
		case LT:
			val = left.Cmp(right) < 0
		case GT:
			val = left.Cmp(right) > 0
		case LTE:
			val = left.Cmp(right) <= 0
		case GTE:
			val = left.Cmp(right) >= 0
		default:
			panic("unreachable")
		}
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.PushBool(val)
		if err != nil {
			return FAULT, err
		}

	case BOOLAND, BOOLOR:
		left, right, err := self.EvalStack.PopPairAsBool()
		if err != nil {
			return FAULT, err
		}

		var val bool
		switch opcode {
		case BOOLAND:
			val = left && right
		case BOOLOR:
			val = left || right
		default:
			panic("unreachable")
		}
		err = self.EvalStack.PushBool(val)
		if err != nil {
			return FAULT, err
		}
	case NOT:
		x, err := self.EvalStack.PopAsBool()
		if err != nil {
			return FAULT, err
		}

		err = self.EvalStack.PushBool(!x)
		if err != nil {
			return FAULT, err
		}
	case WITHIN:
		val, left, right, err := self.EvalStack.PopTripleAsIntVal()
		if err != nil {
			return FAULT, err
		}
		v1 := val.Cmp(left)
		v2 := val.Cmp(right)

		err = self.EvalStack.PushBool(v1 >= 0 && v2 < 0)
		if err != nil {
			return FAULT, err
		}
	case SHA1, SHA256, HASH160, HASH256:
		x, err := self.EvalStack.PopAsBytes()
		if err != nil {
			return FAULT, err
		}

		var hash []byte
		switch opcode {
		case SHA1:
			sh := sha1.New()
			sh.Write(x)
			hash = sh.Sum(nil)
		case SHA256:
			sh := sha256.New()
			sh.Write(x)
			hash = sh.Sum(nil)
		case HASH160:
			temp := sha256.Sum256(x)
			md := ripemd160.New()
			md.Write(temp[:])
			hash = md.Sum(nil)
		case HASH256:
			temp := sha256.Sum256(x)
			data := sha256.Sum256(temp[:])
			hash = data[:]
		}
		val, err := types.VmValueFromBytes(hash)
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}
	case VERIFY:
		pub, sig, data, err := self.EvalStack.PopTripleAsBytes()
		if err != nil {
			return FAULT, err
		}

		key, err := keypair.DeserializePublicKey(pub)
		if err != nil {
			return FAULT, err
		}

		verErr := signature.Verify(key, data, sig)
		err = self.EvalStack.PushBool(verErr == nil)
		if err != nil {
			return FAULT, err
		}
	// Array
	case ARRAYSIZE:
		val, err := self.EvalStack.Pop()
		if err != nil {
			return FAULT, err
		}

		var length int64
		if array, err := val.AsArrayValue(); err == nil {
			length = array.Len()
		} else if buf, err := val.AsBytes(); err == nil {
			length = int64(len(buf))
		} else {
			return FAULT, errors.ERR_BAD_TYPE
		}

		err = self.EvalStack.PushInt64(length)
		if err != nil {
			return FAULT, err
		}
	case PACK:
		size, err := self.EvalStack.PopAsInt64()
		if err != nil {
			return FAULT, err
		}
		if size < 0 {
			return FAULT, errors.ERR_BAD_VALUE
		}
		array := types.NewArrayValue()
		for i := int64(0); i < size; i++ {
			val, err := self.EvalStack.Pop()
			if err != nil {
				return FAULT, err
			}

			array.Append(val)
		}
		err = self.EvalStack.Push(types.VmValueFromArrayVal(array))
		if err != nil {
			return FAULT, err
		}
	case UNPACK:
		arr, err := self.EvalStack.PopAsArray()
		if err != nil {
			return FAULT, err
		}
		l := len(arr.Data)
		for i := l - 1; i >= 0; i-- {
			err = self.EvalStack.Push(arr.Data[i])
			if err != nil {
				return FAULT, err
			}
		}
		err = self.EvalStack.PushInt64(int64(l))
		if err != nil {
			return FAULT, err
		}
	case PICKITEM:
		item, index, err := self.EvalStack.PopPair()
		if err != nil {
			return FAULT, err
		}

		var val types.VmValue
		if array, err := item.AsArrayValue(); err == nil {
			ind, err := index.AsInt64()
			if err != nil {
				return FAULT, err
			}
			if ind < 0 || ind >= array.Len() {
				return FAULT, errors.ERR_INDEX_OUT_OF_BOUND
			}

			val = array.Data[ind]
		} else if struc, err := item.AsStructValue(); err == nil {
			ind, err := index.AsInt64()
			if err != nil {
				return FAULT, err
			}
			if ind < 0 || ind >= struc.Len() {
				return FAULT, errors.ERR_INDEX_OUT_OF_BOUND
			}
			val = struc.Data[ind]
		} else if mapVal, err := item.AsMapValue(); err == nil {
			value, ok, err := mapVal.Get(index)
			if err != nil {
				return FAULT, err
			} else if ok == false {
				// todo: suply a nil value in vm?
				return FAULT, errors.ERR_MAP_NOT_EXIST
			}
			val = value
		} else {
			return FAULT, errors.ERR_BAD_TYPE
		}

		err = self.EvalStack.Push(val)
		if err != nil {
			return FAULT, err
		}

	case SETITEM:
		//todo: the original implementation for Struct type may have problem.
		item, index, val, err := self.EvalStack.PopTriple()
		if err != nil {
			return FAULT, err
		}

		//todo check val is Struct?
		if array, err := item.AsArrayValue(); err == nil {
			ind, err := index.AsInt64()
			if err != nil {
				return FAULT, err
			}
			if ind < 0 || ind >= array.Len() {
				return FAULT, errors.ERR_INDEX_OUT_OF_BOUND
			}

			array.Data[ind] = val
		} else if struc, err := item.AsStructValue(); err == nil {
			ind, err := index.AsInt64()
			if err != nil {
				return FAULT, err
			}
			if ind < 0 || ind >= struc.Len() {
				return FAULT, errors.ERR_INDEX_OUT_OF_BOUND
			}

			struc.Data[ind] = val
		} else if mapVal, err := item.AsMapValue(); err == nil {
			err = mapVal.Set(index, val)
			if err != nil {
				return FAULT, err
			}
		} else {
			return FAULT, errors.ERR_BAD_TYPE
		}
	case NEWARRAY:
		count, err := self.EvalStack.PopAsInt64()
		if err != nil {
			return FAULT, err
		}
		if count < 0 || count > MAX_ARRAY_SIZE {
			return FAULT, errors.ERR_BAD_VALUE
		}
		array := types.NewArrayValue()
		for i := int64(0); i < count; i++ {
			array.Append(types.VmValueFromInt64(0))
		}
		err = self.EvalStack.Push(types.VmValueFromArrayVal(array))
		if err != nil {
			return FAULT, err
		}
	case NEWSTRUCT:
		count, err := self.EvalStack.PopAsInt64()
		if err != nil {
			return FAULT, err
		}
		if count < 0 || count > MAX_ARRAY_SIZE {
			return FAULT, errors.ERR_BAD_VALUE
		}
		array := types.NewStructValue()
		for i := int64(0); i < count; i++ {
			array.Append(types.VmValueFromInt64(0))
		}
		err = self.EvalStack.Push(types.VmValueFromStructVal(array))
		if err != nil {
			return FAULT, err
		}
	case NEWMAP:
		err := self.EvalStack.Push(types.NewMapVmValue())
		if err != nil {
			return FAULT, err
		}
	case APPEND:
		//todo: handle struct
		item, err := self.EvalStack.Pop()
		if err != nil {
			return FAULT, err
		}
		array, err := self.EvalStack.PopAsArray()
		if err != nil {
			return FAULT, err
		}
		array.Append(item)
	case REVERSE:
		array, err := self.EvalStack.PopAsArray()
		if err != nil {
			return FAULT, err
		}

		data := array.Data
		for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
			data[i], data[j] = data[j], data[i]
		}
	case REMOVE:
		item, index, err := self.EvalStack.PopPair()
		if err != nil {
			return FAULT, err
		}
		switch item.GetType() {
		case types.MapType:
			value, err := item.AsMapValue()
			if err == nil {
				return FAULT, err
			}
			err = value.Remove(index)
			if err != nil {
				return FAULT, err
			}
		case types.ArrayType:
			value, err := item.AsArrayValue()
			if err != nil {
				return FAULT, err
			}
			i, err := index.AsInt64()
			if err != nil {
				return FAULT, err
			}
			if i < 0 {
				return FAULT, fmt.Errorf("[REMOVE] index out of bound!")
			}
			err = value.RemoveAt(i + 1)
			if err != nil {
				return FAULT, err
			}
		}
	case HASKEY:
		item, key, err := self.EvalStack.PopPair()
		if err != nil {
			return FAULT, err
		}
		mapValue, err := item.AsMapValue()
		if err != nil {
			return FAULT, err
		}
		_, ok, err := mapValue.Get(key)
		if err != nil {
			return FAULT, err
		}
		err = self.EvalStack.Push(types.VmValueFromBool(ok))
		if err != nil {
			return FAULT, err
		}
	case KEYS:
		item, err := self.EvalStack.Pop()
		if err != nil {
			return FAULT, err
		}
		mapValue, err := item.AsMapValue()
		if err != nil {
			return FAULT, err
		}
		keys, err := mapValue.GetMapSortedKey()
		if err != nil {
			return FAULT, err
		}
		arr := types.NewArrayValue()
		for _, v := range keys {
			t, err := types.VmValueFromBytes([]byte(v))
			if err != nil {
				return FAULT, err
			}
			arr.Append(t)
		}
		err = self.EvalStack.Push(types.VmValueFromArrayVal(arr))
		if err != nil {
			return FAULT, err
		}
	case VALUES:
		item, err := self.EvalStack.Pop()
		if err != nil {
			return FAULT, err
		}
		mapVal, err := item.AsMapValue()
		if err != nil {
			return FAULT, err
		}
		vals, err := mapVal.GetValues()
		arr := types.NewArrayValue()
		for _, v := range vals {
			arr.Append(v)
		}
		err = self.EvalStack.Push(types.VmValueFromArrayVal(arr))
		if err != nil {
			return FAULT, err
		}
	case THROW:
		return FAULT, nil
	case THROWIFNOT:
		val, err := self.EvalStack.PopAsBool()
		if err != nil {
			return FAULT, err
		}
		if !val {
			return FAULT, nil
		}
	default:
		panic("unimplemented!")
	}

	return NONE, nil
}
