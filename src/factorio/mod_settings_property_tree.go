package factorio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	maximumModSettingsFileBytes   = 16 * 1024 * 1024
	maximumModSettingsStringBytes = 1 * 1024 * 1024
	maximumModSettingsNodes       = 100000
	maximumModSettingsDepth       = 64
)

var (
	ErrInvalidModSettingsFile     = errors.New("invalid mod-settings.dat")
	ErrUnsupportedModSettingsFile = errors.New("unsupported mod-settings.dat property type")
)

type propertyTreeType byte

const (
	propertyTreeNone propertyTreeType = iota
	propertyTreeBool
	propertyTreeNumber
	propertyTreeString
	propertyTreeList
	propertyTreeDictionary
	propertyTreeSignedInteger
	propertyTreeUnsignedInteger
)

type propertyTreeEntry struct {
	Name  *string
	Value propertyTreeNode
}

type propertyTreeNode struct {
	Type     propertyTreeType
	AnyType  bool
	Bool     bool
	Number   float64
	String   *string
	Children []propertyTreeEntry
	Signed   int64
	Unsigned uint64
}

type modSettingsDocument struct {
	Version Version
	Root    propertyTreeNode
}

type propertyTreeDecoder struct {
	reader *bytes.Reader
	nodes  int
}

func decodeModSettingsDocument(contents []byte) (modSettingsDocument, error) {
	if len(contents) > maximumModSettingsFileBytes {
		return modSettingsDocument{}, fmt.Errorf("%w: file exceeds %d bytes", ErrInvalidModSettingsFile, maximumModSettingsFileBytes)
	}
	if len(contents) < 11 {
		return modSettingsDocument{}, fmt.Errorf("%w: file is truncated", ErrInvalidModSettingsFile)
	}
	decoder := propertyTreeDecoder{reader: bytes.NewReader(contents)}
	var versionBytes [8]byte
	if _, err := io.ReadFull(decoder.reader, versionBytes[:]); err != nil {
		return modSettingsDocument{}, fmt.Errorf("%w: read version: %v", ErrInvalidModSettingsFile, err)
	}
	version := Version{
		uint(binary.LittleEndian.Uint16(versionBytes[0:2])),
		uint(binary.LittleEndian.Uint16(versionBytes[2:4])),
		uint(binary.LittleEndian.Uint16(versionBytes[4:6])),
		uint(binary.LittleEndian.Uint16(versionBytes[6:8])),
	}
	flag, err := decoder.reader.ReadByte()
	if err != nil {
		return modSettingsDocument{}, fmt.Errorf("%w: read header flag: %v", ErrInvalidModSettingsFile, err)
	}
	if flag != 0 {
		return modSettingsDocument{}, fmt.Errorf("%w: unsupported header flag", ErrInvalidModSettingsFile)
	}
	root, err := decoder.readNode(0)
	if err != nil {
		return modSettingsDocument{}, err
	}
	if decoder.reader.Len() != 0 {
		return modSettingsDocument{}, fmt.Errorf("%w: %d trailing bytes", ErrInvalidModSettingsFile, decoder.reader.Len())
	}
	if root.Type != propertyTreeDictionary {
		return modSettingsDocument{}, fmt.Errorf("%w: root is not a dictionary", ErrInvalidModSettingsFile)
	}
	return modSettingsDocument{Version: version, Root: root}, nil
}

func newModSettingsDocument(version Version) modSettingsDocument {
	return modSettingsDocument{
		Version: version,
		Root: propertyTreeNode{Type: propertyTreeDictionary, Children: []propertyTreeEntry{
			propertyTreeNamedEntry("startup", propertyTreeNode{Type: propertyTreeDictionary}),
			propertyTreeNamedEntry("runtime-global", propertyTreeNode{Type: propertyTreeDictionary}),
			propertyTreeNamedEntry("runtime-per-user", propertyTreeNode{Type: propertyTreeDictionary}),
		}},
	}
}

func propertyTreeNamedEntry(name string, value propertyTreeNode) propertyTreeEntry {
	copyName := name
	return propertyTreeEntry{Name: &copyName, Value: value}
}

func (decoder *propertyTreeDecoder) readNode(depth int) (propertyTreeNode, error) {
	if depth > maximumModSettingsDepth {
		return propertyTreeNode{}, fmt.Errorf("%w: property tree exceeds depth %d", ErrInvalidModSettingsFile, maximumModSettingsDepth)
	}
	decoder.nodes++
	if decoder.nodes > maximumModSettingsNodes {
		return propertyTreeNode{}, fmt.Errorf("%w: property tree exceeds %d nodes", ErrInvalidModSettingsFile, maximumModSettingsNodes)
	}
	typeByte, err := decoder.reader.ReadByte()
	if err != nil {
		return propertyTreeNode{}, fmt.Errorf("%w: read property type: %v", ErrInvalidModSettingsFile, err)
	}
	if typeByte > byte(propertyTreeUnsignedInteger) {
		return propertyTreeNode{}, fmt.Errorf("%w: %d", ErrUnsupportedModSettingsFile, typeByte)
	}
	anyType, err := decoder.readBool("any-type flag")
	if err != nil {
		return propertyTreeNode{}, err
	}
	node := propertyTreeNode{Type: propertyTreeType(typeByte), AnyType: anyType}
	switch node.Type {
	case propertyTreeNone:
	case propertyTreeBool:
		node.Bool, err = decoder.readBool("boolean value")
	case propertyTreeNumber:
		var raw [8]byte
		if _, err = io.ReadFull(decoder.reader, raw[:]); err == nil {
			node.Number = math.Float64frombits(binary.LittleEndian.Uint64(raw[:]))
			if math.IsNaN(node.Number) || math.IsInf(node.Number, 0) {
				err = errors.New("number is not finite")
			}
		}
	case propertyTreeString:
		node.String, err = decoder.readString()
	case propertyTreeList, propertyTreeDictionary:
		var raw [4]byte
		if _, err = io.ReadFull(decoder.reader, raw[:]); err == nil {
			count := binary.LittleEndian.Uint32(raw[:])
			if count > maximumModSettingsNodes || decoder.nodes+int(count) > maximumModSettingsNodes {
				err = fmt.Errorf("property list exceeds %d nodes", maximumModSettingsNodes)
			} else {
				node.Children = make([]propertyTreeEntry, 0, int(count))
				seen := make(map[string]struct{}, int(count))
				for index := uint32(0); index < count; index++ {
					name, readErr := decoder.readString()
					if readErr != nil {
						err = readErr
						break
					}
					if node.Type == propertyTreeDictionary {
						if name == nil {
							err = errors.New("dictionary key is null")
							break
						}
						if _, duplicate := seen[*name]; duplicate {
							err = fmt.Errorf("duplicate dictionary key %q", *name)
							break
						}
						seen[*name] = struct{}{}
					}
					child, readErr := decoder.readNode(depth + 1)
					if readErr != nil {
						err = readErr
						break
					}
					node.Children = append(node.Children, propertyTreeEntry{Name: name, Value: child})
				}
			}
		}
	case propertyTreeSignedInteger:
		var raw [8]byte
		if _, err = io.ReadFull(decoder.reader, raw[:]); err == nil {
			node.Signed = int64(binary.LittleEndian.Uint64(raw[:]))
		}
	case propertyTreeUnsignedInteger:
		var raw [8]byte
		if _, err = io.ReadFull(decoder.reader, raw[:]); err == nil {
			node.Unsigned = binary.LittleEndian.Uint64(raw[:])
		}
	}
	if err != nil {
		return propertyTreeNode{}, fmt.Errorf("%w: decode property type %d: %v", ErrInvalidModSettingsFile, node.Type, err)
	}
	return node, nil
}

func (decoder *propertyTreeDecoder) readBool(label string) (bool, error) {
	value, err := decoder.reader.ReadByte()
	if err != nil {
		return false, fmt.Errorf("%w: read %s: %v", ErrInvalidModSettingsFile, label, err)
	}
	if value > 1 {
		return false, fmt.Errorf("%w: %s is not a boolean", ErrInvalidModSettingsFile, label)
	}
	return value == 1, nil
}

func (decoder *propertyTreeDecoder) readString() (*string, error) {
	isNull, err := decoder.readBool("string null flag")
	if err != nil {
		return nil, err
	}
	if isNull {
		return nil, nil
	}
	lengthByte, err := decoder.reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read string length: %w", err)
	}
	length := uint32(lengthByte)
	if lengthByte == 0xff {
		var raw [4]byte
		if _, err := io.ReadFull(decoder.reader, raw[:]); err != nil {
			return nil, fmt.Errorf("read extended string length: %w", err)
		}
		length = binary.LittleEndian.Uint32(raw[:])
	}
	if length > maximumModSettingsStringBytes || uint64(length) > uint64(decoder.reader.Len()) {
		return nil, fmt.Errorf("string length %d is invalid", length)
	}
	contents := make([]byte, int(length))
	if _, err := io.ReadFull(decoder.reader, contents); err != nil {
		return nil, err
	}
	value := string(contents)
	return &value, nil
}

func (document modSettingsDocument) encode() ([]byte, error) {
	var output bytes.Buffer
	var versionBytes [8]byte
	for index, part := range document.Version {
		if part > math.MaxUint16 {
			return nil, fmt.Errorf("Factorio version component %d exceeds uint16", part)
		}
		binary.LittleEndian.PutUint16(versionBytes[index*2:index*2+2], uint16(part))
	}
	output.Write(versionBytes[:])
	output.WriteByte(0)
	nodes := 0
	if err := writePropertyTreeNode(&output, document.Root, 0, &nodes); err != nil {
		return nil, err
	}
	if output.Len() > maximumModSettingsFileBytes {
		return nil, fmt.Errorf("encoded mod-settings.dat exceeds %d bytes", maximumModSettingsFileBytes)
	}
	return output.Bytes(), nil
}

func writePropertyTreeNode(output *bytes.Buffer, node propertyTreeNode, depth int, nodes *int) error {
	if depth > maximumModSettingsDepth {
		return fmt.Errorf("property tree exceeds depth %d", maximumModSettingsDepth)
	}
	(*nodes)++
	if *nodes > maximumModSettingsNodes {
		return fmt.Errorf("property tree exceeds %d nodes", maximumModSettingsNodes)
	}
	if node.Type > propertyTreeUnsignedInteger {
		return fmt.Errorf("%w: %d", ErrUnsupportedModSettingsFile, node.Type)
	}
	output.WriteByte(byte(node.Type))
	writePropertyTreeBool(output, node.AnyType)
	switch node.Type {
	case propertyTreeNone:
	case propertyTreeBool:
		writePropertyTreeBool(output, node.Bool)
	case propertyTreeNumber:
		if math.IsNaN(node.Number) || math.IsInf(node.Number, 0) {
			return errors.New("property tree number is not finite")
		}
		var raw [8]byte
		binary.LittleEndian.PutUint64(raw[:], math.Float64bits(node.Number))
		output.Write(raw[:])
	case propertyTreeString:
		if err := writePropertyTreeString(output, node.String); err != nil {
			return err
		}
	case propertyTreeList, propertyTreeDictionary:
		if len(node.Children) > maximumModSettingsNodes {
			return fmt.Errorf("property list exceeds %d nodes", maximumModSettingsNodes)
		}
		var count [4]byte
		binary.LittleEndian.PutUint32(count[:], uint32(len(node.Children)))
		output.Write(count[:])
		seen := make(map[string]struct{}, len(node.Children))
		for _, child := range node.Children {
			if node.Type == propertyTreeDictionary {
				if child.Name == nil {
					return errors.New("dictionary key is null")
				}
				if _, duplicate := seen[*child.Name]; duplicate {
					return fmt.Errorf("duplicate dictionary key %q", *child.Name)
				}
				seen[*child.Name] = struct{}{}
			}
			if err := writePropertyTreeString(output, child.Name); err != nil {
				return err
			}
			if err := writePropertyTreeNode(output, child.Value, depth+1, nodes); err != nil {
				return err
			}
		}
	case propertyTreeSignedInteger:
		var raw [8]byte
		binary.LittleEndian.PutUint64(raw[:], uint64(node.Signed))
		output.Write(raw[:])
	case propertyTreeUnsignedInteger:
		var raw [8]byte
		binary.LittleEndian.PutUint64(raw[:], node.Unsigned)
		output.Write(raw[:])
	}
	return nil
}

func writePropertyTreeBool(output *bytes.Buffer, value bool) {
	if value {
		output.WriteByte(1)
		return
	}
	output.WriteByte(0)
}

func writePropertyTreeString(output *bytes.Buffer, value *string) error {
	if value == nil {
		output.WriteByte(1)
		return nil
	}
	output.WriteByte(0)
	contents := []byte(*value)
	if len(contents) > maximumModSettingsStringBytes {
		return fmt.Errorf("property tree string exceeds %d bytes", maximumModSettingsStringBytes)
	}
	if len(contents) < 0xff {
		output.WriteByte(byte(len(contents)))
	} else {
		output.WriteByte(0xff)
		var raw [4]byte
		binary.LittleEndian.PutUint32(raw[:], uint32(len(contents)))
		output.Write(raw[:])
	}
	output.Write(contents)
	return nil
}

func (document *modSettingsDocument) setStartupValue(name string, value propertyTreeNode) error {
	startup, err := dictionaryChild(&document.Root, "startup", true)
	if err != nil {
		return err
	}
	setting, err := dictionaryChild(startup, name, true)
	if err != nil {
		return err
	}
	for index := range setting.Children {
		if setting.Children[index].Name != nil && *setting.Children[index].Name == "value" {
			value.AnyType = setting.Children[index].Value.AnyType
			setting.Children[index].Value = value
			return nil
		}
	}
	setting.Children = append(setting.Children, propertyTreeNamedEntry("value", value))
	return nil
}

func dictionaryChild(parent *propertyTreeNode, name string, create bool) (*propertyTreeNode, error) {
	if parent.Type != propertyTreeDictionary {
		return nil, fmt.Errorf("%w: expected dictionary while locating %q", ErrInvalidModSettingsFile, name)
	}
	for index := range parent.Children {
		if parent.Children[index].Name != nil && *parent.Children[index].Name == name {
			if parent.Children[index].Value.Type != propertyTreeDictionary {
				return nil, fmt.Errorf("%w: %q is not a dictionary", ErrInvalidModSettingsFile, name)
			}
			return &parent.Children[index].Value, nil
		}
	}
	if !create {
		return nil, osErrNotExist{name: name}
	}
	parent.Children = append(parent.Children, propertyTreeNamedEntry(name, propertyTreeNode{Type: propertyTreeDictionary}))
	return &parent.Children[len(parent.Children)-1].Value, nil
}

// osErrNotExist is an internal sentinel that avoids treating a missing optional
// PropertyTree branch as a malformed file.
type osErrNotExist struct{ name string }

func (err osErrNotExist) Error() string { return err.name + " does not exist" }
