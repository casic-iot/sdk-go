package numberx

import (
	"reflect"
	"testing"
)

func TestGetValueByType(t *testing.T) {
	type args struct {
		valueType FieldType
		v         interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    interface{}
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "float1",
			args: args{
				valueType: Float,
				v:         "1",
			},
			want:    float64(1),
			wantErr: false,
		},
		{
			name: "float2",
			args: args{
				valueType: Float,
				v:         "true",
			},
			want:    float64(1),
			wantErr: false,
		},
		{
			name: "int1",
			args: args{
				valueType: Int,
				v:         "1",
			},
			want:    1,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetValueByType(tt.args.valueType, tt.args.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetValueByType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetValueByType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBytesToFloat16(t *testing.T) {
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		args    args
		want    float32
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "1",
			args: args{
				b: []byte{0x00, 0x07},
			},
			want:    -0,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BytesToFloat16(tt.args.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("BytesToFloat16() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("BytesToFloat16() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFloat16ToBytes(t *testing.T) {
	type args struct {
		f float32
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Float16ToBytes(tt.args.f)
			if (err != nil) != tt.wantErr {
				t.Errorf("Float16ToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Float16ToBytes() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFloat16(t *testing.T) {
	b := []byte{0x00, 0x01}
	f, err := BytesToFloat16(b)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := Float16ToBytes(f)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(bytes)
}
