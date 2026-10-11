//go:build windows

package saved

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func encryptPayload(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return plaintext, nil
	}
	var inBlob windows.DataBlob
	inBlob.Size = uint32(len(plaintext))
	inBlob.Data = &plaintext[0]

	var outBlob windows.DataBlob
	err := windows.CryptProtectData(&inBlob, nil, nil, 0, nil, 0, &outBlob)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))

	res := make([]byte, outBlob.Size)
	copy(res, unsafe.Slice(outBlob.Data, outBlob.Size))
	return res, nil
}

func decryptPayload(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return ciphertext, nil
	}
	var inBlob windows.DataBlob
	inBlob.Size = uint32(len(ciphertext))
	inBlob.Data = &ciphertext[0]

	var outBlob windows.DataBlob
	err := windows.CryptUnprotectData(&inBlob, nil, nil, 0, nil, 0, &outBlob)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))

	res := make([]byte, outBlob.Size)
	copy(res, unsafe.Slice(outBlob.Data, outBlob.Size))
	return res, nil
}
