package eml

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sort"
	"sync"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
)

// The fuzz targets corrupt real PST files: the input is a list of patches (5 bytes each: a 4-byte index into the
// offsets read by an unpatched run, and the new byte value), so the corpus contains no PST content.
// They call the reader and the converter directly, without the panic recovery of Walk and Convert,
// so panics are found instead of being reported as errors.
//
//	go test ./pkg/eml/ -run '^$' -fuzz FuzzEnron -fuzztime 5m

// fuzzMaxMessages limits the messages read per input.
const fuzzMaxMessages = 30

// fuzzMaxPatches limits the patches per input.
const fuzzMaxPatches = 64

type fuzzBase struct {
	data []byte
	// readOffsets are the offsets read by an unpatched run.
	readOffsets []int64
}

var (
	fuzzBases     = map[string]*fuzzBase{}
	fuzzBasesLock sync.Mutex
)

// getFuzzBase reads the PST file and records the offsets an unpatched run reads.
func getFuzzBase(f *testing.F, path string) *fuzzBase {
	fuzzBasesLock.Lock()
	defer fuzzBasesLock.Unlock()

	if base, ok := fuzzBases[path]; ok {
		return base
	}

	data, err := os.ReadFile(path)

	if errors.Is(err, os.ErrNotExist) {
		f.Skipf("%s not found", path)
	} else if err != nil {
		f.Fatal(err)
	}

	recorder := &recordingReader{data: data, offsets: map[int64]bool{}}

	readPST(recorder)

	base := &fuzzBase{data: data}

	for offset := range recorder.offsets {
		base.readOffsets = append(base.readOffsets, offset)
	}

	sort.Slice(base.readOffsets, func(i, j int) bool { return base.readOffsets[i] < base.readOffsets[j] })

	fuzzBases[path] = base

	return base
}

// recordingReader records the offsets read.
type recordingReader struct {
	data    []byte
	offsets map[int64]bool
}

func (recordingReader *recordingReader) ReadAt(buffer []byte, offset int64) (int, error) {
	if offset < 0 || offset >= int64(len(recordingReader.data)) {
		return 0, io.EOF
	}

	n := copy(buffer, recordingReader.data[offset:])

	for i := int64(0); i < int64(n); i++ {
		recordingReader.offsets[offset+i] = true
	}

	if n < len(buffer) {
		return n, io.EOF
	}

	return n, nil
}

// patchedReader reads the base data with patches applied.
type patchedReader struct {
	data    []byte
	patches map[int64]byte
}

func (patchedReader *patchedReader) ReadAt(buffer []byte, offset int64) (int, error) {
	if offset < 0 || offset >= int64(len(patchedReader.data)) {
		return 0, io.EOF
	}

	n := copy(buffer, patchedReader.data[offset:])

	for patchOffset, value := range patchedReader.patches {
		if patchOffset >= offset && patchOffset < offset+int64(n) {
			buffer[patchOffset-offset] = value
		}
	}

	if n < len(buffer) {
		return n, io.EOF
	}

	return n, nil
}

// readPST opens the file and converts its first messages, as Walk and Convert do but without recovering panics.
func readPST(reader io.ReaderAt) {
	pstFile, err := pst.New(reader)

	if err != nil {
		return
	}

	messageCount := 0

	_ = pstFile.WalkFolders(func(folder *pst.Folder) error {
		if messageCount >= fuzzMaxMessages {
			return io.EOF
		}

		messageIterator, err := folder.GetMessageIterator()

		if err != nil {
			return nil
		}

		for messageCount < fuzzMaxMessages && messageIterator.Next() {
			messageCount++

			converter := &converter{result: &Result{}}

			_ = converter.writeMessage(io.Discard, messageIterator.Value(), 0)
		}

		return nil
	})
}

func fuzzPST(f *testing.F, path string) {
	base := getFuzzBase(f, path)

	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 0, 0xFF})

	f.Fuzz(func(t *testing.T, input []byte) {
		patches := map[int64]byte{}

		for i := 0; i+5 <= len(input) && len(patches) < fuzzMaxPatches; i += 5 {
			index := binary.LittleEndian.Uint32(input[i : i+4])
			patches[base.readOffsets[int(index%uint32(len(base.readOffsets)))]] = input[i+4]
		}

		readPST(&patchedReader{data: base.data, patches: patches})
	})
}

func FuzzEnron(f *testing.F) {
	fuzzPST(f, "../../data/enron.pst")
}

func FuzzANSI(f *testing.F) {
	fuzzPST(f, "../../data/32-bit.pst")
}

// FuzzPrivate corrupts the private test PST (see PrivateTestPST): HTML and RTF bodies, attachments,
// embedded messages. Its corpus holds only patch offsets and values, no content of the file.
func FuzzPrivate(f *testing.F) {
	fuzzPST(f, PrivateTestPST(f))
}
