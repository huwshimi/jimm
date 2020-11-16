// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	jujuparams "github.com/juju/juju/apiserver/params"
)

// Strings is a data type that stores a slice of strings into a single
// column. The strings are encoded as a JSON array and stored in a BLOB
// data type.
type Strings []string

// GormDataType implements schema.GormDataTypeInterface.
func (s Strings) GormDataType() string {
	return "bytes"
}

// Value implements driver.Valuer.
func (s Strings) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan implements sql.Scanner.
func (s *Strings) Scan(src interface{}) error {
	if src == nil {
		*s = nil
		return nil
	}
	var buf []byte
	switch v := src.(type) {
	case []byte:
		buf = v
	case string:
		buf = []byte(v)
	default:
		return fmt.Errorf("cannot unmarshal %T as Strings", src)
	}
	return json.Unmarshal(buf, s)
}

// A StringMap is a data type that flattens a map of string to string into
// a single column. The map is encoded as a JSON object and stored in a
// BLOB data type.
type StringMap map[string]string

// GormDataType implements schema.GormDataTypeInterface.
func (m StringMap) GormDataType() string {
	return "bytes"
}

// Value implements driver.Valuer.
func (m StringMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner.
func (m *StringMap) Scan(src interface{}) error {
	if src == nil {
		*m = nil
		return nil
	}
	var buf []byte
	switch v := src.(type) {
	case []byte:
		buf = v
	case string:
		buf = []byte(v)
	default:
		return fmt.Errorf("cannot unmarshal %T as StringMap", src)
	}
	return json.Unmarshal(buf, m)
}

// A Map stores a generic map in a database column. The map is encoded as
// JSON and stored in a BLOB element.
type Map map[string]interface{}

// GormDataType implements schema.GormDataTypeInterface.
func (m Map) GormDataType() string {
	return "bytes"
}

// Value implements driver.Valuer.
func (m Map) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner.
func (m *Map) Scan(src interface{}) error {
	if src == nil {
		*m = nil
		return nil
	}
	var buf []byte
	switch v := src.(type) {
	case []byte:
		buf = v
	case string:
		buf = []byte(v)
	default:
		return fmt.Errorf("cannot unmarshal %T as Map", src)
	}
	return json.Unmarshal(buf, m)
}

// HostPorts is data type that stores a set of jujuparams.HostPort in a
// single column. The hostports are encoded as JSON and stored in a BLOB
// value.
type HostPorts [][]jujuparams.HostPort

// GormDataType implements schema.GormDataTypeInterface.
func (HostPorts) GormDataType() string {
	return "bytes"
}

// Value implements driver.Valuer.
func (hp HostPorts) Value() (driver.Value, error) {
	if hp == nil {
		return nil, nil
	}
	// It would normally be bad practice to directly encode exernal
	// data-types one doesn't control in the database, but in this case
	// it is probalbly fine because it is part of the published API and
	// therefore is unlikely to change in an incompatible way.
	return json.Marshal(hp)
}

// Scan implements sql.Scanner.
func (hp *HostPorts) Scan(src interface{}) error {
	if src == nil {
		*hp = nil
		return nil
	}
	var buf []byte
	switch v := src.(type) {
	case []byte:
		buf = v
	case string:
		buf = []byte(v)
	default:
		return fmt.Errorf("cannot unmarshal %T as HostPorts", src)
	}
	return json.Unmarshal(buf, hp)
}