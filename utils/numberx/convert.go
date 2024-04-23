package numberx

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strconv"
)

type FieldType string

const (
	String  FieldType = "string"
	Float   FieldType = "float"
	Int     FieldType = "integer"
	Bool    FieldType = "boolean"
	UNKNOWN FieldType = "UNKNOWN"
)

func (v FieldType) String() string {
	switch v {
	case String:
		return "string"
	case Float:
		return "float"
	case Int:
		return "integer"
	case Bool:
		return "boolean"
	default:
		return "UNKNOWN"
	}
}

func GetValueByType(valueType FieldType, v interface{}) (interface{}, error) {
	if reflect.TypeOf(v).Kind() == reflect.Ptr {
		v = reflect.ValueOf(v).Elem().Interface()
	}
	switch valueType {
	case String:
		val, err := GetString(v)
		if err != nil {
			return nil, err
		}
		return val, nil
	case Float:
		val, err := GetFloat(v)
		if err != nil {
			return nil, err
		}
		return val, nil
	case Int:
		val, err := GetInt(v)
		if err != nil {
			return nil, err
		}
		return val, nil
	case Bool:
		val, err := GetBool(v)
		if err != nil {
			return nil, err
		}
		return val, nil
	default:
		switch r := v.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
			return GetFloat(r)
		case string:
			return r, nil
		//case bool:
		//	if r {
		//		return 1, nil
		//	} else {
		//		return 0, nil
		//	}
		default:
			return nil, fmt.Errorf("数据类型非int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, string, bool")
		}
	}
}

func GetString(v interface{}) (string, error) {
	switch r := v.(type) {
	case int:
		return strconv.Itoa(r), nil
	case int8:
		return strconv.FormatInt(int64(r), 10), nil
	case int16:
		return strconv.FormatInt(int64(r), 10), nil
	case int32:
		return strconv.FormatInt(int64(r), 10), nil
	case int64:
		return strconv.FormatInt(r, 10), nil
	case uint:
		return strconv.FormatUint(uint64(r), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(r), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(r), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(r), 10), nil
	case uint64:
		return strconv.FormatUint(r, 10), nil
	case float32:
		return strconv.FormatFloat(float64(r), 'f', -1, 64), nil
	case float64:
		return strconv.FormatFloat(r, 'f', -1, 32), nil
	case string:
		return r, nil
	case bool:
		if r {
			return "1", nil
		} else {
			return "0", nil
		}
	}
	return "", fmt.Errorf("不能转字符串,数据类型未知或错误")
}

func GetFloat(v interface{}) (float64, error) {
	switch r := v.(type) {
	case int:
		return float64(r), nil
	case int8:
		return float64(r), nil
	case int16:
		return float64(r), nil
	case int32:
		return float64(r), nil
	case int64:
		return float64(r), nil
	case uint:
		return float64(r), nil
	case uint8:
		return float64(r), nil
	case uint16:
		return float64(r), nil
	case uint32:
		return float64(r), nil
	case uint64:
		return float64(r), nil
	case float32:
		return float64(r), nil
	case float64:
		return r, nil
	case string:
		s, err := strconv.ParseFloat(r, 64)
		if err != nil {
			return 0, fmt.Errorf("转换值 %s 到 %s 错误,%s", v, Float, err)
		}
		return s, nil
	case bool:
		if r {
			return float64(1), nil
		} else {
			return float64(0), nil
		}
	}
	return 0, fmt.Errorf("不能转浮点数,数据类型未知或错误")
}

func GetInt(v interface{}) (int, error) {
	switch r := v.(type) {
	case int:
		return r, nil
	case int8:
		return int(r), nil
	case int16:
		return int(r), nil
	case int32:
		return int(r), nil
	case int64:
		return int(r), nil
	case uint:
		return int(r), nil
	case uint8:
		return int(r), nil
	case uint16:
		return int(r), nil
	case uint32:
		return int(r), nil
	case uint64:
		return int(r), nil
	case float32:
		return int(r), nil
	case float64:
		return int(r), nil
	case string:
		s, err := strconv.Atoi(r)
		if err != nil {
			return 0, fmt.Errorf("转换值 %s 到 %s 错误,%s", v, Int, err)
		}
		return s, nil
	case bool:
		if r {
			return 1, nil
		} else {
			return 0, nil
		}
	}
	return 0, fmt.Errorf("不能转整型,数据类型未知或错误")
}

func GetBool(v interface{}) (int, error) {
	switch r := v.(type) {
	case int:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case int8:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case int16:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case int32:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case int64:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case uint:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case uint8:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case uint16:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case uint32:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case uint64:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case float32:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case float64:
		if r != 0 {
			return 1, nil
		} else {
			return 0, nil
		}
	case string:
		if r == "true" || r == "false" {
			b, err := strconv.ParseBool(r)
			if err != nil {
				return 0, fmt.Errorf("转换值 %s 到 %s 错误,%s", v, Bool, err)
			}
			if b {
				return 1, nil
			} else {
				return 0, nil
			}

		} else {
			s, err := strconv.ParseFloat(r, 64)
			if err != nil {
				return 0, fmt.Errorf("转换值 %s 解析浮点到 %s 错误,%s", v, Bool, err)
			}
			if s != 0 {
				return 1, nil
			} else {
				return 0, nil
			}
		}
	case bool:
		if r {
			return 1, nil
		} else {
			return 0, nil
		}
	}
	return 0, fmt.Errorf("不能转布尔类型,数据类型未知或错误")
}

// BytesToFloat16 将二进制形式的字节数组转换为对应的16位浮点数。
func BytesToFloat16(b []byte) (float32, error) {
	if len(b) != 2 {
		return 0, fmt.Errorf("invalid length: needed 2 bytes but got %d", len(b))
	}

	// 假设字节是大端序编排。
	bits := binary.BigEndian.Uint16(b)

	// 单独提取符号、指数和尾数位。
	sign := bits >> 15
	exp := (bits >> 10) & 0x1F
	frac := bits & 0x3FF

	// 处理不同情况。
	switch exp {
	case 0:
		if frac == 0 {
			return float32(0), nil // 零值
		}
		// 非规格数 (subnormal)
		return float32(-1+int(sign)*2) * float32(math.Pow(2, -14)) * (float32(frac) / 1024), nil
	case 0x1F: // 指数位全为1处理无穷和NaN
		if frac == 0 {
			if sign == 0 {
				return float32(math.Inf(1)), nil // 正无穷
			}
			return float32(math.Inf(-1)), nil // 负无穷
		}
		return float32(math.NaN()), nil // NaN
	default:
		// 规格化数值
		mantissa := 1 + float32(frac)/1024
		exponent := math.Pow(2, float64(exp)-15)
		return float32(-1+int(sign)*2) * mantissa * float32(exponent), nil
	}
}

// Float16ToBytes 将16位浮点数转换为对应的二进制形式字节数组。
func Float16ToBytes(f float32) ([]byte, error) {
	if f == 0 {
		return []byte{0x00, 0x00}, nil
	}

	if math.IsNaN(float64(f)) {
		// Use a general pattern for NaN
		return []byte{0x7E, 0x00}, nil
	}

	if math.IsInf(float64(f), 1) {
		// Positive infinity
		return []byte{0x7C, 0x00}, nil
	}

	if math.IsInf(float64(f), -1) {
		// Negative infinity
		return []byte{0xFC, 0x00}, nil
	}

	sign := uint16(0)
	if f < 0 {
		sign = 1 << 15
		f = -f // Take the absolute value for processing
	}

	absf := float64(f)
	exp := int(math.Floor(math.Log2(absf))) + 15
	mantf := absf/math.Pow(2, float64(exp-15)) - 1

	if exp <= 0 {
		// Handle subnormal numbers
		mantf = absf / math.Pow(2, -14)
		exp = 0
	} else if exp >= 31 {
		// Handle overflow by returning an error
		return nil, fmt.Errorf("value %v is out of range for Float16", float64(f))
	}

	mant := uint16(mantf*1024) & 0x03FF

	// Now pack the sign, exp, and mantissa into a 16-bit format.
	bits := sign | uint16(exp<<10&0x7C00) | mant
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, bits)
	return b, nil
}
