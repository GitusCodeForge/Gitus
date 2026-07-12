package gitlib

import (
	"errors"
	"io"
	"os"
	"regexp"
)

func intToHex16(i int) string {
	res := make([]byte, 0)
	res = append(res, digitToChar(byte((i>>12)&0xf)))
	res = append(res, digitToChar(byte((i>>8)&0xf)))
	res = append(res, digitToChar(byte((i>>4)&0xf)))
	res = append(res, digitToChar(byte((i)&0xf)))
	return string(res)
}

func byteHexToInt(s string) int {
	s1 := 0
	if int('a') <= int(s[0]) && int(s[0]) <= int('f') {
		s1 = int(s[0]) - int('a') + 10
	} else if int('A') <= int(s[0]) && int(s[0]) <= int('F') {
		s1 = int(s[0]) - int('A') + 10
	} else {
		s1 = int(s[0]) - int('0')
	}
	s2 := 0
	if int('a') <= int(s[1]) && int(s[1]) <= int('f') {
		s2 = int(s[1]) - int('a') + 10
	} else if int('A') <= int(s[1]) && int(s[1]) <= int('F') {
		s2 = int(s[1]) - int('A') + 10
	} else {
		s2 = int(s[1]) - int('0')
	}
	return s1 * 16 + s2
}

func digitToChar(c byte) byte {
	if c >= 0x0a {
		return c + byte('a') - 10
	} else { return c + byte('0') }
}
func charHexToDigit(c byte) byte {
	if c >= byte('a') && c <= byte('f') { return byte(int(c) - int('a') + 10) }
	if c >= byte('A') && c <= byte('F') { return byte(int(c) - int('A') + 10) }
	if c >= byte('0') && c <= byte('9') { return byte(int(c) - int('0')) }
	return 0
}
func readBigEndianUInt32(f *os.File) (uint32, error) {
	var int32buf []byte = make([]byte, 4)
	_, err := io.ReadFull(f, int32buf)
	if err != nil { return 0, err }
	byte1 := uint32(int32buf[0])
	byte2 := uint32(int32buf[1])
	byte3 := uint32(int32buf[2])
	byte4 := uint32(int32buf[3])
	return (byte1<<24)|(byte2<<16)|(byte3<<8)|byte4, nil
}
func readBigEndianUInt64(f *os.File) (uint64, error) {
	var int64buf []byte = make([]byte, 8)
	_, err := io.ReadFull(f, int64buf)
	if err != nil { return 0, err }
	byte1 := uint64(int64buf[0])
	byte2 := uint64(int64buf[1])
	byte3 := uint64(int64buf[2])
	byte4 := uint64(int64buf[3])
	byte5 := uint64(int64buf[4])
	byte6 := uint64(int64buf[5])
	byte7 := uint64(int64buf[6])
	byte8 := uint64(int64buf[7])
	return (byte1<<56)|(byte2<<48)|(byte3<<40)|(byte4<<32)|(byte5<<24)|(byte6<<16)|(byte7<<8)|byte8, nil
}

// reads `l` bytes and returns the hex string of it.
// all hex are lowercase.
func readBytesToHex(f io.Reader, l int) (string, error) {
	buf := make([]byte, l)
	n, err := io.ReadFull(f, buf)
	if err != nil { return "", err }
	// all characters of hex numbers are within single-byte range
	// thus this should be correct.
	res := make([]byte, 2*n)
	for i := range n {
		res[2*i] = digitToChar(buf[i]>>4)
		res[2*i+1] = digitToChar(buf[i]&0x0f)
	}
	return string(res), err
}

func readZeroTerminatedString(f io.Reader) (string, error) {
	buf := make([]byte, 1)
	resbuf := make([]byte, 0)
	for {
		_, err := io.ReadFull(f, buf)
		if err != nil { return "", err }
		if int(buf[0]) == 0 { break }
		resbuf = append(resbuf, buf[0])
	}
	return string(resbuf), nil
}

func readUntil(f io.Reader, c byte) ([]byte, error) {
	bytebuf := make([]byte, 1)
	res := make([]byte, 0)
	for {
		_, err := io.ReadFull(f, bytebuf)
		if err != nil { return nil, err }
		if int(bytebuf[0]) == int(c) { break }
		res = append(res, bytebuf[0])
	}
	return res, nil
}

var ErrInvalidHexString = errors.New("Invalid hex string")
func hexStringToBytes(s string) []byte {
	// NOTE: all invalid hex digits are interpreted as zero.
	res := make([]byte, 0)
	i := 0
	l := len(s)
	if l % 2 > 0 {
		res = append(res, charHexToDigit(s[0]))
		i += 1
	}
	for i < l {
		ch1 := charHexToDigit(s[i])
		ch2 := charHexToDigit(s[i+1])
		res = append(res, (ch1 << 4) | ch2)
		i += 2
	}
	return res
}

var REGEX_HEX_STRING = regexp.MustCompile("^[0-9a-fA-F]+$")
func IsValidSHA1(s string) bool {
	return REGEX_HEX_STRING.MatchString(s) && len(s) == 40
}

func IsValidSHA256(s string) bool {
	return REGEX_HEX_STRING.MatchString(s) && len(s) == 64
}

func IsValidId(s string) bool {
	return IsValidSHA1(s) || IsValidSHA256(s)
}

