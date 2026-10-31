package gitlib

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strconv"
)

type GitObjectType int
const (
	INVALID GitObjectType = 0
	COMMIT GitObjectType = 1
	TREE GitObjectType = 2
	BLOB GitObjectType = 3
	TAG GitObjectType = 4
	OFS_DELTA GitObjectType = 6
	REF_DELTA GitObjectType = 7
)

type GitObject interface {
	Type() GitObjectType
	ObjectId() string
	RawData() []byte
}

func (got GitObjectType) String() string {
	switch got {
	case COMMIT: return "COMMIT"
	case TREE: return "TREE"
	case BLOB: return "BLOB"
	case TAG: return "TAG"
	case OFS_DELTA: return "OFS_DELTA"
	case REF_DELTA: return "REF_DELTA"
	default: log.Panicf("Invalid object type: %d", int(got))
	}
	return ""
}

type RawGitObject struct {
	objId string
	objType GitObjectType
	objSize int64
	reader io.ReadCloser
	// we need to have this because directly accessible objects and
	// packed objects both use the same RawGitObject datatype in our
	// codebase but their structure is different.
	// A directly accessible object is "Deflate(ObjHeader+ObjData)",
	// but a packed object is "ObjHeader+Deflate(ObjData)";
	// This means that when we reach to the point of parsing ObjData
	// we don't know we are within "Deflate()" or not.
	// As for the reason why we don't enter "Deflate()" in both cases:
	// delta objects have extra info after ObjHeader but before the
	// call to Deflate (so it's like "ObjHeader+ExtraStuff+Deflate()";
	// we need to parse the extra stuff as if the reader is not a
	// zlib reader.
	readerIsUncompressed bool
	// associated pack index for OFS_DELTA resolving.
	// nil for directly accessible objects.
	packIndex *PackIndex
	packOffset int64
}

type GitObjectHeader struct {
	Type GitObjectType
	Size int64
}

func EncodeAsDirectObject(gobj GitObject) ([]byte, error) {
	// requires `gobj` to be resolved (i.e. not delta)
	typestr := ""
	switch gobj.Type() {
	case TREE: typestr = "tree"
	case COMMIT: typestr = "commit"
	case BLOB: typestr = "blob"
	case TAG: typestr = "tag"
	default: return nil, errors.New("Invalid type for encoding: " + gobj.Type().String())
	}
	size := fmt.Sprintf("%d", len(gobj.RawData()))
	header := []byte(typestr + " " + size + "\x00")
	res := append(header, gobj.RawData()...)
	return res, nil
}

func EncodeAsDirectObjectCompressed(gobj GitObject) ([]byte, error) {
	preres, err := EncodeAsDirectObject(gobj)
	if err != nil { return nil, err }
	var res bytes.Buffer
	wr := zlib.NewWriter(&res)
	wr.Write(preres)
	wr.Flush()
	wr.Close()
	return res.Bytes(), nil
}

// returns the type and size of the object.
func parseDirectlyAccessibleObjectHeader(s io.Reader) (GitObjectHeader, error) {
	typeBytes, err := readUntil(s, byte(' '))
	if err != nil { return GitObjectHeader{}, nil }
	typeStr := string(typeBytes)
	sizeBytes, err := readUntil(s, byte(0))
	if err != nil { return GitObjectHeader{}, nil }
	size, err := strconv.ParseInt(string(sizeBytes), 10, 64)
	typenum := INVALID
	switch typeStr {
	case "tree": typenum = TREE
	case "blob": typenum = BLOB
	case "tag": typenum = TAG
	case "commit": typenum = COMMIT
	default: return GitObjectHeader{}, errors.New("Invalid object type in header")
	}
	return GitObjectHeader{Type: typenum, Size: size}, nil
}

func (gr LocalGitRepository) openRawDirectlyAccessibleObject(oid string) (RawGitObject, error) {
	objStorePath := path.Join(gr.GitDirectoryPath, "objects")
	objPath := path.Join(objStorePath, oid[:2], oid[2:])
	f, err := os.Open(objPath)
	if err != nil { return RawGitObject{}, err }
	nr, err := zlib.NewReader(f)
	if err != nil { return RawGitObject{}, err }
	objHead, err := parseDirectlyAccessibleObjectHeader(nr)
	if err != nil { return RawGitObject{}, err }
	res := RawGitObject{
		objId: oid,
		objType: objHead.Type,
		objSize: objHead.Size,
		reader: nr,
		readerIsUncompressed: true,
	}
	return res, nil
}

func (gr LocalGitRepository) openRawObject(oid string) (RawGitObject, error) {
	dao, err := gr.openRawDirectlyAccessibleObject(oid)
	if err == nil { return dao, err }
	for _, val := range gr.PackIndex {
		o, err := val.openPackedObject(oid)
		if err != nil { continue }
		return o, nil
	}
	return RawGitObject{}, errors.New("Object not found")
}

func (rgo RawGitObject) dispatch() (GitObject, error) {
	switch rgo.objType {
	case TREE:
		t, err := rgo.ReadAsTreeObject()
		if err != nil { return nil, err }
		return t, nil
	case COMMIT:
		t, err := rgo.ReadAsCommitObject()
		if err != nil { return nil, err }
		return t, nil
	case TAG:
		t, err := rgo.ReadAsTagObject()
		if err != nil { return nil, err }
		return t, nil
	case BLOB:
		t, err := rgo.ReadAsBlobObject()
		if err != nil { return nil, err }
		return t, nil
	case REF_DELTA:
		t, err := rgo.ReadAsRefDeltaObject()
		if err != nil { return nil, err }
		return t, nil
	case OFS_DELTA:
		t, err := rgo.ReadAsOfsDeltaObject()
		if err != nil { return nil, err }
		return t, nil
	default:
		return nil, errors.New("Invalid raw git object type")
	}
}

func (rgo RawGitObject) dispatchNoDeflate() (GitObject, error) {
	switch rgo.objType {
	case TREE:
		t, err := rgo.readAsTreeObjectNoDeflate()
		if err != nil { return nil, err }
		return t, nil
	case COMMIT:
		t, err := rgo.readAsCommitObjectNoDeflate()
		if err != nil { return nil, err }
		return t, nil
	case TAG:
		t, err := rgo.readAsTagObjectNoDeflate()
		if err != nil { return nil, err }
		return t, nil
	case BLOB:
		t, err := rgo.readAsBlobObjectNoDeflate()
		if err != nil { return nil, err }
		return t, nil
	default:
		return nil, errors.New("Invalid raw git object type")
	}
}

func (gr LocalGitRepository) ReadObject(oid string) (GitObject, error) {
	rgo, err := gr.openRawObject(oid)
	if err != nil { return nil, err }
	defer rgo.reader.Close()
	var dispatched GitObject = nil
	if rgo.readerIsUncompressed {
		// NOTE: this is safe (note that dispatchNoDeflate does not handle
		// delta objects) because delta objects will not occur as
		// directly accessible objects and only as packed object, and if we
		// are dealing with packed objects (opened thru openPackedObject)
		// we won't have `rgo.readerIsUncompressed = true`.
		dispatched, err = rgo.dispatchNoDeflate()
	} else {
		dispatched, err = rgo.dispatch()
	}
	if err != nil { return nil, err }
	resolved, err := gr.resolveObject(dispatched)
	return resolved, err
}

func (gr LocalGitRepository) ReadObjectNoResolve(oid string) (GitObject, error) {
	rgo, err := gr.openRawObject(oid)
	if err != nil { return nil, err }
	defer rgo.reader.Close()
	var dispatched GitObject = nil
	if rgo.readerIsUncompressed {
		dispatched, err = rgo.dispatchNoDeflate()
	} else {
		dispatched, err = rgo.dispatch()
	}
	if err != nil { return nil, err }
	return dispatched, err
}


type bytesReader struct {
	r *bytes.Reader
}

func (br bytesReader) Read(f []byte) (int, error) {
	return br.r.Read(f)
}
func (br bytesReader) Close() error {
	return nil
}

func (gr LocalGitRepository) resolveObject(obj GitObject) (GitObject, error) {
	var commandList []DeltaCommand = nil
	var baseObj GitObject = nil
	var packIndex *PackIndex = nil
	switch obj.Type() {
	case REF_DELTA:
		dobj := obj.(*RefDeltaObject)
		commandList = dobj.CommandList
		packIndex = dobj.PackIndex
		baseRO, err := gr.openRawObject(dobj.BaseObjectId)
		if err != nil { return nil, err }
		if baseRO.objType == REF_DELTA || baseRO.objType == OFS_DELTA {
			dispatched, err := baseRO.dispatch()
			if err != nil { return nil, err }
			baseObj, err = gr.resolveObject(dispatched)
			if err != nil { return nil, err}
		} else {
			baseObj, err = baseRO.dispatch()
			if err != nil { return nil, err }
		}
	case OFS_DELTA:
		dobj := obj.(*OfsDeltaObject)
		commandList = dobj.CommandList
		packIndex = dobj.PackIndex
		pf, err := dobj.PackIndex.openPackFile()
		if err != nil { return nil, err }
		defer pf.Close()
		off := dobj.BaseObjectOffset
		goh, err := parsePackedObjectHeaderAtOffset(pf, off)
		if err != nil { return nil, err }
		baseRO := RawGitObject{
			objType: goh.Type,
			objSize: goh.Size,
			packIndex: dobj.PackIndex,
			reader: pf,
			packOffset: off,
		}
		if baseRO.objType == REF_DELTA || baseRO.objType == OFS_DELTA {
			dispatched, err := baseRO.dispatch()
			if err != nil { return nil, err }
			baseObj, err = gr.resolveObject(dispatched)
			if err != nil { return nil, err }
		} else {
			baseObj, err = baseRO.dispatch()
			if err != nil { return nil, err }
		}
	default:
		return obj, nil
	}
	
	// append into one buffer: slices.Concat reallocated and copied the whole
	// result for every command, which is quadratic on long delta chains.
	res := make([]byte, 0, 256)
	for _, cmd := range commandList {
		res = append(res, cmd.Execute(baseObj.RawData())...)
	}
	br := bytesReader{r: bytes.NewReader(res)}
	resrgo := RawGitObject{
		objId: obj.ObjectId(),
		objType: baseObj.Type(),
		objSize: int64(len(res)),
		reader: br,
		packIndex: packIndex,
	}
	resObj, err := resrgo.dispatchNoDeflate()
	if err != nil { return nil, err }
	return resObj, nil
}


