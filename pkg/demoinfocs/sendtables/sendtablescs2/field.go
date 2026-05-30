package sendtablescs2

import (
	"fmt"
	"strconv"

	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
)

const (
	fieldModelSimple = iota
	fieldModelFixedArray
	fieldModelFixedTable
	fieldModelVariableArray
	fieldModelVariableTable
)

// polyUpdate is returned by the base decoder of a polymorphic fixed-table field.
// id is the field's polySerializerId; ser is the newly selected serializer
// (nil when the pointer is inactive / boolean was false).
type polyUpdate struct {
	id  int
	ser *serializer
}

type field struct {
	varName           string
	varType           string
	sendNode          string
	serializerName    string
	serializerVersion int32
	encoder           string
	encodeFlags       *int32
	bitCount          *int32
	lowValue          *float32
	highValue         *float32
	fieldType         *fieldType
	serializer        *serializer
	model             int
	polyTypes         []*serializer
	// polySerializerId is ≥ 0 for polymorphic fixed-table fields; -1 otherwise.
	// It indexes into the per-entity polySerializers slice, so the active
	// serializer is tracked per entity rather than on the shared field object.
	polySerializerId int

	decoder      fieldDecoder
	baseDecoder  fieldDecoder
	childDecoder fieldDecoder
}

func newField(serializers map[string]*serializer, ser *msg.CSVCMsg_FlattenedSerializer, f *msg.ProtoFlattenedSerializerFieldT) *field {
	resolve := func(p *int32) string {
		if p == nil {
			return ""
		}
		return ser.GetSymbols()[*p]
	}

	x := &field{
		varName:           resolve(f.VarNameSym),
		varType:           resolve(f.VarTypeSym),
		sendNode:          resolve(f.SendNodeSym),
		serializerName:    resolve(f.FieldSerializerNameSym),
		serializerVersion: f.GetFieldSerializerVersion(),
		encoder:           resolve(f.VarEncoderSym),
		encodeFlags:       f.EncodeFlags,
		bitCount:          f.BitCount,
		lowValue:          f.LowValue,
		highValue:         f.HighValue,
		model:             fieldModelSimple,
		polySerializerId:  -1,
	}

	if len(f.PolymorphicTypes) > 0 {
		// Build combined slice: [0] = default/field serializer, [1..N] = polymorphic alternatives.
		// The ubitvar read from the bitstream is a direct index into this slice, where
		// 0 selects the field's own serializer and 1..N select the polymorphic variants.
		x.polyTypes = make([]*serializer, len(f.PolymorphicTypes)+1)
		x.polyTypes[0] = serializers[resolve(f.FieldSerializerNameSym)]

		for i, t := range f.PolymorphicTypes {
			x.polyTypes[i+1] = serializers[resolve(t.PolymorphicFieldSerializerNameSym)] //nolint:gosec
		}
	}

	if x.sendNode == "(root)" {
		x.sendNode = ""
	}

	return x
}

func (f *field) setModel(model int) {
	f.model = model

	switch model {
	case fieldModelFixedArray:
		f.decoder = findDecoder(f)

	case fieldModelFixedTable:
		if len(f.polyTypes) == 0 {
			// Fixed pointer: single serializer, never changes type.
			// Only a boolean is read from the stream; serializer is on the field.
			f.baseDecoder = booleanDecoder
		} else {
			// Polymorphic pointer: bool then (if true) a ubitvar type index.
			// Returns a *polyUpdate so the active serializer can be stored
			// per entity rather than on this shared field object.
			polyTypes := f.polyTypes
			polyId := f.polySerializerId
			f.baseDecoder = func(r *reader) any {
				if r.readBoolean() {
					return &polyUpdate{id: polyId, ser: polyTypes[r.readUBitVar()]}
				}
				return &polyUpdate{id: polyId, ser: nil}
			}
		}

	case fieldModelVariableArray:
		if f.fieldType.genericType == nil {
			_panicf("no generic type for variable array field %#v", f)
		}
		f.baseDecoder = unsignedDecoder
		f.childDecoder = findDecoderByBaseType(f)

	case fieldModelVariableTable:
		f.baseDecoder = unsignedDecoder

	case fieldModelSimple:
		f.decoder = findDecoder(f)
	}
}

func (f *field) getNameForFieldPath(fp *fieldPath, pos int, ps []*serializer) []string {
	x := []string{f.varName}

	switch f.model {
	case fieldModelFixedArray:
		if fp.last == pos {
			x = append(x, fmt.Sprintf("%04d", fp.path[pos]))
		}

	case fieldModelFixedTable:
		if fp.last >= pos {
			ser := f.serializer
			if f.polySerializerId >= 0 && ps != nil {
				ser = ps[f.polySerializerId]
			}
			if ser != nil {
				x = append(x, ser.getNameForFieldPath(fp, pos, ps)...)
			}
		}

	case fieldModelVariableArray:
		if fp.last == pos {
			x = append(x, fmt.Sprintf("%04d", fp.path[pos]))
		}

	case fieldModelVariableTable:
		if fp.last != pos-1 {
			x = append(x, fmt.Sprintf("%04d", fp.path[pos]))
			if fp.last != pos {
				x = append(x, f.serializer.getNameForFieldPath(fp, pos+1, ps)...)
			}
		}
	}

	return x
}

// getDecoderAndCollection returns the decoder and whether this field path is a
// variable-length collection update that requires fieldState handling.
// This encodes the (base && variableArray|variableTable) check directly,
// avoiding a separate getFieldForFieldPath traversal.
func (f *field) getDecoderAndCollection(fp *fieldPath, pos int, ps []*serializer) (fieldDecoder, bool) {
	switch f.model {
	case fieldModelFixedArray:
		return f.decoder, false

	case fieldModelFixedTable:
		if fp.last == pos-1 {
			return f.baseDecoder, false // base decoder but fixed, no fieldState update
		}
		ser := f.serializer
		if f.polySerializerId >= 0 && ps != nil {
			ser = ps[f.polySerializerId]
		}
		if ser == nil {
			return nil, false // polymorphic pointer not yet activated
		}
		return ser.getDecoderAndCollection(fp, pos, ps)

	case fieldModelVariableArray:
		if fp.last == pos {
			return f.childDecoder, false
		}

		return f.baseDecoder, true // variable collection update

	case fieldModelVariableTable:
		if fp.last >= pos+1 {
			return f.serializer.getDecoderAndCollection(fp, pos+1, ps)
		}

		return f.baseDecoder, true // variable collection update
	}

	return f.decoder, false
}

func (f *field) getFieldPathForName(fp *fieldPath, name string, ps []*serializer) bool {
	switch f.model {
	case fieldModelFixedArray:
		assertLen(name, 4)
		fp.path[fp.last] = mustAtoi(name)
		return true

	case fieldModelFixedTable:
		ser := f.serializer
		if f.polySerializerId >= 0 && ps != nil {
			ser = ps[f.polySerializerId]
		}
		if ser == nil {
			return false
		}
		return ser.getFieldPathForName(fp, name, ps)

	case fieldModelVariableArray:
		assertLen(name, 4)
		fp.path[fp.last] = mustAtoi(name)
		return true

	case fieldModelVariableTable:
		assertLenMin(name, 6)
		fp.path[fp.last] = mustAtoi(name[:4])
		fp.last++
		return f.serializer.getFieldPathForName(fp, name[5:], ps)

	case fieldModelSimple:
		_panicf("not supported")
	}

	return false
}

func (f *field) getFieldPaths(fp *fieldPath, state *fieldState, ps []*serializer) []*fieldPath { //nolint:gocognit
	x := make([]*fieldPath, 0, 1)

	switch f.model {
	case fieldModelFixedArray:
		if sub, ok := state.get(fp).(*fieldState); ok {
			fp.last++
			for i, v := range sub.state {
				if v != nil {
					fp.path[fp.last] = i
					x = append(x, fp.copy())
				}
			}
			fp.last--
		}

	case fieldModelFixedTable:
		if sub, ok := state.get(fp).(*fieldState); ok {
			ser := f.serializer
			if f.polySerializerId >= 0 && ps != nil {
				ser = ps[f.polySerializerId]
			}
			if ser != nil {
				fp.last++
				x = append(x, ser.getFieldPaths(fp, sub, ps)...)
				fp.last--
			}
		}

	case fieldModelVariableArray:
		if sub, ok := state.get(fp).(*fieldState); ok {
			fp.last++
			for i, v := range sub.state {
				if v != nil {
					fp.path[fp.last] = i
					x = append(x, fp.copy())
				}
			}
			fp.last--
		}

	case fieldModelVariableTable:
		if sub, ok := state.get(fp).(*fieldState); ok {
			fp.last += 2
			for i, v := range sub.state {
				if vv, ok := v.(*fieldState); ok {
					fp.path[fp.last-1] = i
					x = append(x, f.serializer.getFieldPaths(fp, vv, ps)...)
				}
			}
			fp.last -= 2
		}

	case fieldModelSimple:
		x = append(x, fp.copy())
	}

	return x
}

func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		_panicf("assertion failed: '%s' not a number", s)
	}
	return n
}

func assertLen(s string, n int) {
	if len(s) != n {
		_panicf("assertion failed: '%s' is not %d long", s, n)
	}
}

func assertLenMin(s string, n int) {
	if len(s) < n {
		_panicf("assertion failed: '%s' is less than %d long", s, n)
	}
}
