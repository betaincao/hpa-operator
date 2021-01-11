package wrapper

import "testing"

func Test_mapSubset(t *testing.T) {
	type args struct {
		source map[string]string
		subset map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			want: true,
			args: args{
				source: map[string]string{
					"country": "china",
					"foo":     "bar",
				},
				subset: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapSubset(tt.args.source, tt.args.subset); got != tt.want {
				t.Errorf("mapSubset() = %v, want %v", got, tt.want)
			}
		})
	}
}
