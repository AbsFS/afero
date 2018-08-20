package mem

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fatih/structtag"
)

func TestFileDataNameRace(t *testing.T) {
	t.Parallel()
	const someName = "someName"
	const someOtherName = "someOtherName"
	d := FileData{
		name: someName,
	}

	if d.Name() != someName {
		t.Errorf("Failed to read correct Name, was %v", d.Name())
	}

	ChangeFileName(&d, someOtherName)
	if d.Name() != someOtherName {
		t.Errorf("Failed to set Name, was %v", d.Name())
	}

	go func() {
		ChangeFileName(&d, someName)
	}()

	if d.Name() != someName && d.Name() != someOtherName {
		t.Errorf("Failed to read either Name, was %v", d.Name())
	}
}

func w(t *testing.T) {
	t.Parallel()
	someTime := time.Now()
	someOtherTime := someTime.Add(1 * time.Minute)

	d := FileData{
		modtime: someTime,
	}

	s := FileInfo{
		FileData: &d,
	}

	if s.ModTime() != someTime {
		t.Errorf("Failed to read correct value, was %v", s.ModTime())
	}

	SetModTime(&d, someOtherTime)
	if s.ModTime() != someOtherTime {
		t.Errorf("Failed to set ModTime, was %v", s.ModTime())
	}

	go func() {
		SetModTime(&d, someTime)
	}()

	if s.ModTime() != someTime && s.ModTime() != someOtherTime {
		t.Errorf("Failed to read either modtime, was %v", s.ModTime())
	}
}

func TestFileDataModeRace(t *testing.T) {
	t.Parallel()
	const someMode = 0777
	const someOtherMode = 0660

	d := FileData{
		mode: someMode,
	}

	s := FileInfo{
		FileData: &d,
	}

	if s.Mode() != someMode {
		t.Errorf("Failed to read correct value, was %v", s.Mode())
	}

	SetMode(&d, someOtherMode)
	if s.Mode() != someOtherMode {
		t.Errorf("Failed to set Mode, was %v", s.Mode())
	}

	go func() {
		SetMode(&d, someMode)
	}()

	if s.Mode() != someMode && s.Mode() != someOtherMode {
		t.Errorf("Failed to read either mode, was %v", s.Mode())
	}
}

func TestFileDataIsDirRace(t *testing.T) {
	t.Parallel()

	d := FileData{
		mode: os.ModeDir,
	}

	s := FileInfo{
		FileData: &d,
	}

	if s.IsDir() != true {
		t.Errorf("Failed to read correct value, was %v", s.IsDir())
	}

	go func() {
		s.Lock()
		d.mode = 0
		s.Unlock()
	}()

	//just logging the value to trigger a read:
	t.Logf("Value is %v", s.IsDir())
}

func TestFileDataSizeRace(t *testing.T) {
	t.Parallel()

	const someData = "Hello"
	const someOtherDataSize = "Hello World"

	d := FileData{
		data: []byte(someData),
		mode: 0644,
	}

	s := FileInfo{
		FileData: &d,
	}

	if s.Size() != int64(len(someData)) {
		t.Errorf("Failed to read correct value. Expected %d, got %v", len(someData), s.Size())
	}

	go func() {
		s.Lock()
		d.data = []byte(someOtherDataSize)
		s.Unlock()
	}()

	//just logging the value to trigger a read:
	t.Logf("Value is %v", s.Size())

	//Testing the Dir size case
	d.mode = d.mode | os.ModeDir
	if s.Size() != int64(42) {
		t.Errorf("Failed to read correct value for dir, was %v", s.Size())
	}
}

func TestFile(t *testing.T) {
	const data = "hello, world\n"

	tests := []struct {
		Op string `test:"op"`

		Offset int64  `test:"input,Seek,WriteAt,ReadAt,Truncate"`
		Whence int    `test:"input,Seek"`
		Input  []byte `test:"input,Write,WriteAt,WriteString"`

		// expected results
		ExpOutput []byte `test:"exp,Read,ReadAt"`
		ExpN      int    `test:"exp,Read,ReadAt,Write,WriteAt,WriteString"`
		ExpErr    error  `test:"exp,Read,ReadAt,Write,WriteAt,WriteString,Seek,Truncate"`
		ExpPos    int64  `test:"exp,Seek"`
	}{
		// Read, Write, Seek
		{Op: "WriteString", Input: []byte("Hello, World\n"), ExpN: 13},
		{Op: "Read", ExpOutput: make([]byte, 2), ExpErr: io.EOF},
		{Op: "Seek", Offset: -1, Whence: io.SeekCurrent, ExpPos: 12},
		{Op: "Read", ExpOutput: []byte("\n"), ExpN: 1},
		{Op: "Seek", Offset: 0, Whence: io.SeekStart, ExpPos: 0},
		{Op: "Read", ExpOutput: []byte("Hello, World\n"), ExpN: 13},
		{Op: "Seek", Offset: 0, Whence: io.SeekStart, ExpPos: 0},
		{Op: "Write", Input: []byte("c"), ExpN: 1},
		{Op: "Read", ExpOutput: []byte("ello, World\n"), ExpN: 12},
		{Op: "Seek", Offset: -1, Whence: io.SeekCurrent, ExpPos: 12},
		{Op: "Write", Input: []byte("!\n"), ExpN: 2},
		{Op: "Seek", Offset: 0, Whence: io.SeekStart, ExpPos: 0},
		{Op: "Read", ExpOutput: []byte("cello, World!\n"), ExpN: 14},

		// WriteAt
		{Op: "Seek", Offset: 0, Whence: io.SeekStart, ExpPos: 0},
		{Op: "WriteAt", Input: []byte("WORLD"), Offset: 7, ExpN: 5},
		{Op: "Seek", Offset: 0, Whence: io.SeekCurrent, ExpPos: 0},
		{Op: "Read", ExpOutput: []byte("cello, WORLD!\n"), ExpN: 14},

		// ReadAt
		{Op: "Seek", Offset: 0, Whence: io.SeekStart, ExpPos: 0},
		{Op: "ReadAt", ExpOutput: []byte("llo, WO"), Offset: 2, ExpN: 7},
		{Op: "Seek", Offset: 0, Whence: io.SeekCurrent, ExpPos: 0},

		// Truncate
		{Op: "Truncate", Offset: 10},
		{Op: "Seek", Offset: 0, Whence: io.SeekEnd, ExpPos: 10},
		{Op: "Truncate", Offset: 1024},
		{Op: "Seek", Offset: 0, Whence: io.SeekEnd, ExpPos: 1024},
		{Op: "Truncate", Offset: 0},
		{Op: "Seek", Offset: 0, Whence: io.SeekEnd, ExpPos: 0},
	}

	filename := os.TempDir()
	name := "testfile.txt"
	// f, err := ioutil.TempFile(filename, "afero")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// name :=f.Name()
	filename = filepath.Join(filename, name)

	f := NewFileHandle(CreateFile(filename))
	defer func() {
		f.Close()
		os.RemoveAll(filename)
	}()

	for i, test := range tests {
		var n int
		var pos int64
		var err error
		var output []byte

		if test.ExpOutput != nil {
			output = make([]byte, len(test.ExpOutput))
		}

		switch test.Op {
		case "Read":
			n, err = f.Read(output)
		case "ReadAt":
			n, err = f.ReadAt(output, test.Offset)
		case "Write":
			n, err = f.Write(test.Input)
		case "Seek":
			pos, err = f.Seek(test.Offset, test.Whence)
		case "Truncate":
			err = f.Truncate(test.Offset)
		case "WriteAt":
			n, err = f.WriteAt(test.Input, test.Offset)
		case "WriteString":
			n, err = f.WriteString(string(test.Input))
		}

		expected := getExpectations(i, t, test)

		// Tests: Seek
		if expected["ExpPos"] && pos != test.ExpPos {
			t.Errorf("%s %d: pos: %d, expected: %d\n", test.Op, i, pos, test.ExpPos)
		}

		// Tests: Read, Write, WriteAt, Seek, Truncate
		if expected["ExpErr"] && err != test.ExpErr {
			t.Errorf("%s %d: error: %v, expected: %v\n", test.Op, i, err, test.ExpErr)
		}

		// Tests: Read, Write
		if expected["ExpN"] && n != test.ExpN {
			t.Errorf("%s %d: n: %d, expected: %d\n", test.Op, i, n, n)
		}

		// Tests: Read
		if expected["ExpOutput"] && bytes.Compare(output, test.ExpOutput) != 0 {
			t.Errorf("%s %d: output: %q, expected: %q\n", test.Op, i, output, test.ExpOutput)
		}
	}
}

func getExpectations(i int, t *testing.T, test interface{}) map[string]bool {
	var inputs []string
	expected := make(map[string]bool)
	reftype := reflect.TypeOf(test)
	var opfield, op string

	for j := 0; j < reftype.NumField(); j++ {
		field := reftype.FieldByIndex([]int{j})

		tags, err := structtag.Parse(string(field.Tag))
		if err != nil {
			t.Fatal(err)
		}

		testTag, err := tags.Get("test")
		if err != nil {
			t.Fatal(err)
		}

		if testTag.Name == "op" {
			opfield = field.Name
			break
		}
	}

	if len(opfield) == 0 {
		t.Fatalf("test struct must have `test:\"op\"` tag")
	}
	refval := reflect.ValueOf(test)

	val := refval.FieldByName(opfield)
	op = val.String()

	for j := 0; j < reftype.NumField(); j++ {
		field := reftype.FieldByIndex([]int{j})

		tags, err := structtag.Parse(string(field.Tag))
		if err != nil {
			t.Fatal(err)
		}

		// get a single tag
		testTag, err := tags.Get("test")
		if err != nil {
			t.Fatal(err)
		}

		active := false
		for _, oppt := range testTag.Options {
			if oppt == op {
				active = true
				break
			}
		}
		if !active {
			continue
		}

		if testTag.Name == "exp" {
			expected[field.Name] = true
		}
		if testTag.Name == "input" {
			name := field.Name
			i := refval.FieldByName(name).Interface()
			val := fmt.Sprintf("%v", i)
			switch v := i.(type) {
			case string:
				val = fmt.Sprintf("%q", v)
			case []byte:
				val = fmt.Sprintf("%q", string(v))
			}
			inputs = append(inputs, fmt.Sprintf("%s:%s", name, val))
		}

	}

	var explist []string
	for k := range expected {
		explist = append(explist, k)
	}
	sort.Strings(explist)
	sort.Strings(inputs)

	t.Logf("%d: Test %s( %s ) ( %s )", i, op, strings.Join(inputs, ", "), strings.Join(explist, ", "))
	return expected
}
