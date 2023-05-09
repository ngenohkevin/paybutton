package payments

import (
	"bytes"
	"github.com/skip2/go-qrcode"
	"image/png"
	"os"
)

func GenerateQRCode(address string, filename string) error {
	qrCode, err := qrcode.Encode(address, qrcode.Medium, 256)
	if err != nil {
		return err
	}

	img, err := png.Decode(bytes.NewReader(qrCode))
	if err != nil {
		return err
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {

		}
	}(file)

	err = png.Encode(file, img)
	if err != nil {
		return err
	}

	return nil
}
