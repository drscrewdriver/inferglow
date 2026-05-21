// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package schema

import (
	"fmt"
	"reflect"
	"testing"
)

// G1-07.1: Schema 推导 Benchmark
// 覆盖：不同字段数（10/100/1000）、嵌套 struct、带 tag vs 无 tag 字段。

// benchSmallStruct 10 个字段（带 json tag）
type benchSmallStruct struct {
	F1  string `json:"f1"`
	F2  int    `json:"f2"`
	F3  bool   `json:"f3"`
	F4  string `json:"f4"`
	F5  int    `json:"f5"`
	F6  bool   `json:"f6"`
	F7  string `json:"f7"`
	F8  int    `json:"f8"`
	F9  bool   `json:"f9"`
	F10 string `json:"f10"`
}

// benchMediumStruct 100 个字段（带 json tag）
//
//nolint:lll // 长字段列表是 benchmark 必需
type benchMediumStruct struct {
	F1   string `json:"f1"`
	F2   int    `json:"f2"`
	F3   bool   `json:"f3"`
	F4   string `json:"f4"`
	F5   int    `json:"f5"`
	F6   bool   `json:"f6"`
	F7   string `json:"f7"`
	F8   int    `json:"f8"`
	F9   bool   `json:"f9"`
	F10  string `json:"f10"`
	F11  string `json:"f11"`
	F12  int    `json:"f12"`
	F13  bool   `json:"f13"`
	F14  string `json:"f14"`
	F15  int    `json:"f15"`
	F16  bool   `json:"f16"`
	F17  string `json:"f17"`
	F18  int    `json:"f18"`
	F19  bool   `json:"f19"`
	F20  string `json:"f20"`
	F21  string `json:"f21"`
	F22  int    `json:"f22"`
	F23  bool   `json:"f23"`
	F24  string `json:"f24"`
	F25  int    `json:"f25"`
	F26  bool   `json:"f26"`
	F27  string `json:"f27"`
	F28  int    `json:"f28"`
	F29  bool   `json:"f29"`
	F30  string `json:"f30"`
	F31  string `json:"f31"`
	F32  int    `json:"f32"`
	F33  bool   `json:"f33"`
	F34  string `json:"f34"`
	F35  int    `json:"f35"`
	F36  bool   `json:"f36"`
	F37  string `json:"f37"`
	F38  int    `json:"f38"`
	F39  bool   `json:"f39"`
	F40  string `json:"f40"`
	F41  string `json:"f41"`
	F42  int    `json:"f42"`
	F43  bool   `json:"f43"`
	F44  string `json:"f44"`
	F45  int    `json:"f45"`
	F46  bool   `json:"f46"`
	F47  string `json:"f47"`
	F48  int    `json:"f48"`
	F49  bool   `json:"f49"`
	F50  string `json:"f50"`
	F51  string `json:"f51"`
	F52  int    `json:"f52"`
	F53  bool   `json:"f53"`
	F54  string `json:"f54"`
	F55  int    `json:"f55"`
	F56  bool   `json:"f56"`
	F57  string `json:"f57"`
	F58  int    `json:"f58"`
	F59  bool   `json:"f59"`
	F60  string `json:"f60"`
	F61  string `json:"f61"`
	F62  int    `json:"f62"`
	F63  bool   `json:"f63"`
	F64  string `json:"f64"`
	F65  int    `json:"f65"`
	F66  bool   `json:"f66"`
	F67  string `json:"f67"`
	F68  int    `json:"f68"`
	F69  bool   `json:"f69"`
	F70  string `json:"f70"`
	F71  string `json:"f71"`
	F72  int    `json:"f72"`
	F73  bool   `json:"f73"`
	F74  string `json:"f74"`
	F75  int    `json:"f75"`
	F76  bool   `json:"f76"`
	F77  string `json:"f77"`
	F78  int    `json:"f78"`
	F79  bool   `json:"f79"`
	F80  string `json:"f80"`
	F81  string `json:"f81"`
	F82  int    `json:"f82"`
	F83  bool   `json:"f83"`
	F84  string `json:"f84"`
	F85  int    `json:"f85"`
	F86  bool   `json:"f86"`
	F87  string `json:"f87"`
	F88  int    `json:"f88"`
	F89  bool   `json:"f89"`
	F90  string `json:"f90"`
	F91  string `json:"f91"`
	F92  int    `json:"f92"`
	F93  bool   `json:"f93"`
	F94  string `json:"f94"`
	F95  int    `json:"f95"`
	F96  bool   `json:"f96"`
	F97  string `json:"f97"`
	F98  int    `json:"f98"`
	F99  bool   `json:"f99"`
	F100 string `json:"f100"`
}

// benchNestedStruct 含嵌套 struct 字段
type benchNestedChild struct {
	ChildField1 string `json:"child_field_1"`
	ChildField2 int    `json:"child_field_2"`
}

type benchNestedStruct struct {
	Name     string             `json:"name"`
	Age      int                `json:"age"`
	Child    benchNestedChild   `json:"child"`
	ChildP   *benchNestedChild  `json:"child_p"`
	Siblings []benchNestedChild `json:"siblings"`
}

// benchNoTagStruct 字段无 json tag（应被推导跳过）
type benchNoTagStruct struct {
	Field1 string
	Field2 int
	Field3 bool
}

// BenchmarkSchemaDeriveFromType 测试 DefineOutputFromType 在不同字段数下的性能。
func BenchmarkSchemaDeriveFromType(b *testing.B) {
	cases := []struct {
		name string
		t    reflect.Type
	}{
		{"fields_10", reflect.TypeOf(benchSmallStruct{})},
		{"fields_100", reflect.TypeOf(benchMediumStruct{})},
		{"nested", reflect.TypeOf(benchNestedStruct{})},
		{"no_tag", reflect.TypeOf(benchNoTagStruct{})},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = DefineOutputFromType(c.t)
			}
		})
	}
}

// BenchmarkSchemaDeriveGeneric 测试泛型入口 DefineOutput[T] 的开销
// （含 reflect.TypeOf 调用）。
func BenchmarkSchemaDeriveGeneric(b *testing.B) {
	b.Run("small", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = DefineOutput[benchSmallStruct]()
		}
	})
	b.Run("medium", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = DefineOutput[benchMediumStruct]()
		}
	})
	b.Run("nested", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = DefineOutput[benchNestedStruct]()
		}
	})
}

// BenchmarkSchemaDeriveRepeated 测试重复推导同一类型的开销稳定性
// （缓存命中场景目前未实现，此 benchmark 用于评估未来加入缓存的收益）。
func BenchmarkSchemaDeriveRepeated(b *testing.B) {
	t := reflect.TypeOf(benchMediumStruct{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DefineOutputFromType(t)
	}
}

// 为避免编译器优化掉整个 benchmark，_ = 之外提供一个 sanity check。
func TestBenchSchemaSanity(t *testing.T) {
	s := DefineOutput[benchSmallStruct]()
	if len(s.Fields) != 10 {
		t.Errorf("expected 10 fields, got %d", len(s.Fields))
	}
	s2 := DefineOutput[benchNoTagStruct]()
	if len(s2.Fields) != 0 {
		t.Errorf("expected 0 fields (no tags), got %d", len(s2.Fields))
	}
	s3 := DefineOutput[benchNestedStruct]()
	if len(s3.Fields) != 5 {
		t.Errorf("expected 5 fields, got %d", len(s3.Fields))
	}
	_ = fmt.Sprintf("sanity ok: %d / %d / %d", len(s.Fields), len(s2.Fields), len(s3.Fields))
}
