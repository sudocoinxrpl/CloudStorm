// -------------------- nft/nft.go --------------------

// license generation functionalities

package nft

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"log"

	"github.com/skip2/go-qrcode"
)

const (
	CardWidth     = 600
	CardHeight    = 800
	HeaderHeight  = 80
	BarcodeHeight = 120
)

type ImageData struct {
	Width, Height int
	Pixels        []uint32
}

func NewImageData(w, h int) ImageData {
	return ImageData{Width: w, Height: h, Pixels: make([]uint32, w*h)}
}

func FillGradient(img *ImageData, topColor, bottomColor uint32) {
	for y := 0; y < img.Height; y++ {
		t := float64(y) / float64(img.Height-1)
		a := uint8(float64((topColor>>24)&0xFF)*(1-t) + float64((bottomColor>>24)&0xFF)*t)
		r := uint8(float64((topColor>>16)&0xFF)*(1-t) + float64((bottomColor>>16)&0xFF)*t)
		g := uint8(float64((topColor>>8)&0xFF)*(1-t) + float64((bottomColor>>8)&0xFF)*t)
		b := uint8(float64(topColor&0xFF)*(1-t) + float64(bottomColor&0xFF)*t)
		col := (uint32(a) << 24) | (uint32(r) << 16) | (uint32(g) << 8) | uint32(b)
		for x := 0; x < img.Width; x++ {
			img.Pixels[y*img.Width+x] = col
		}
	}
}

func ComputeNFTHex(txID, cid, consensusServiceID, combinedProof string) string {
	h := sha256.New()
	h.Write([]byte(txID + cid + consensusServiceID + combinedProof))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateNFTTradingCard(txID, cid, license, qrData, issuer, consensusServiceID, combinedProof string) ImageData {
	nftHex := ComputeNFTHex(txID, cid, consensusServiceID, combinedProof)
	topColor := uint32(0xFF8A2BE2)
	bottomColor := uint32(0xFFFF8C00)
	if qrData == "" {
		qrData = txID + "_" + cid + "_" + consensusServiceID + "_" + combinedProof
	}
	qr, err := qrcode.New(qrData, qrcode.High)
	if err != nil {
		// fallback to blank
		qr = &qrcode.QRCode{}
	}
	qrBitmap := qr.Bitmap()
	qrSize := len(qrBitmap)
	scale := 10
	qrImg := NewImageData(qrSize*scale, qrSize*scale)
	for y := 0; y < qrSize; y++ {
		for x := 0; x < qrSize; x++ {
			var col uint32
			if qrBitmap[y][x] {
				col = 0xFF000000
			} else {
				col = 0xFFFFFFFF
			}
			for yy := 0; yy < scale; yy++ {
				for xx := 0; xx < scale; xx++ {
					qrImg.Pixels[(y*scale+yy)*qrImg.Width+(x*scale+xx)] = col
				}
			}
		}
	}
	img := NewImageData(CardWidth, CardHeight)
	FillGradient(&img, topColor, bottomColor)
	startX := (CardWidth - qrImg.Width) / 2
	startY := (CardHeight - qrImg.Height) / 2
	for y := 0; y < qrImg.Height; y++ {
		for x := 0; x < qrImg.Width; x++ {
			img.Pixels[(startY+y)*img.Width+(startX+x)] = qrImg.Pixels[y*qrImg.Width+x]
		}
	}
	_ = nftHex
	_ = license
	_ = issuer
	_ = consensusServiceID
	_ = combinedProof
	return img
}

func ImageDataToNRGBA(img ImageData) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, img.Width, img.Height))
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			col := img.Pixels[y*img.Width+x]
			a := uint8(col >> 24)
			r := uint8((col >> 16) & 0xFF)
			g := uint8((col >> 8) & 0xFF)
			b := uint8(col & 0xFF)
			out.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}
	return out
}

// The following are placeholders that can be further expanded to verify XRPL address ownership:
func VerifyNFTLicense(licenseNFTCID, issuer string) bool {
	// placeholder logic
	return true
}

func VerifyRippleAddressOwnership(rippleAddress, issuer string) bool {
	// placeholder logic
	return true
}

func IssueNFT(issuer, licenseCID string) error {
	// placeholder logic
	log.Printf("NFT minted for issuer=%s with licenseCID=%s", issuer, licenseCID)
	return nil
}
