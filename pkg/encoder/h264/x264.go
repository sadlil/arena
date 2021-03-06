package h264

// #include <stdlib.h>
import "C"
import (
	"fmt"
	"log"

	x264 "github.com/sergystepanov/x264-go/v2/x264c/external"
)

type H264 struct {
	ref *x264.T

	width      int32
	lumaSize   int32
	chromaSize int32
	csp        int32
	nnals      int32
	nals       []*x264.Nal

	// keep monotonic pts to suppress warnings
	pts int64
}

func NewEncoder(width, height int, options ...Option) (encoder *H264, err error) {
	libVersion := int(x264.Build)

	if libVersion < 150 {
		return nil, fmt.Errorf("x264: the library version should be newer than v150, you have got version %v", libVersion)
	}

	if libVersion < 160 {
		log.Printf("x264: warning, installed version of libx264 %v is older than minimally supported v160, expect bugs", libVersion)
	}

	opts := &Options{
		Crf:     12,
		Tune:    "zerolatency",
		Preset:  "superfast",
		Profile: "baseline",
	}

	for _, opt := range options {
		opt(opts)
	}

	if opts.LogLevel > 0 {
		log.Printf("x264: build v%v", x264.Build)
	}

	param := x264.Param{}
	if opts.Preset != "" && opts.Tune != "" {
		if x264.ParamDefaultPreset(&param, opts.Preset, opts.Tune) < 0 {
			return nil, fmt.Errorf("x264: invalid preset/tune name")
		}
	} else {
		x264.ParamDefault(&param)
	}

	if opts.Profile != "" {
		if x264.ParamApplyProfile(&param, opts.Profile) < 0 {
			return nil, fmt.Errorf("x264: invalid profile name")
		}
	}

	// legacy encoder lacks of this param
	param.IBitdepth = 8

	if libVersion > 155 {
		param.ICsp = x264.CspI420
	} else {
		param.ICsp = 1
	}
	param.IWidth = int32(width)
	param.IHeight = int32(height)
	param.ILogLevel = opts.LogLevel

	param.Rc.IRcMethod = x264.RcCrf
	param.Rc.FRfConstant = float32(opts.Crf)

	encoder = &H264{
		csp:        param.ICsp,
		lumaSize:   int32(width * height),
		chromaSize: int32(width*height) / 4,
		nals:       make([]*x264.Nal, 1),
		width:      int32(width),
	}

	var picIn x264.Picture
	x264.PictureInit(&picIn)

	if encoder.ref = x264.EncoderOpen(&param); encoder.ref == nil {
		err = fmt.Errorf("x264: cannot open the encoder")
		return
	}
	return
}

func (e *H264) Encode(yuv []byte) []byte {
	var picIn, picOut x264.Picture

	picIn.Img.ICsp = e.csp
	picIn.Img.IPlane = 3
	picIn.Img.IStride[0] = e.width
	picIn.Img.IStride[1] = e.width / 2
	picIn.Img.IStride[2] = e.width / 2

	picIn.Img.Plane[0] = C.CBytes(yuv[:e.lumaSize])
	picIn.Img.Plane[1] = C.CBytes(yuv[e.lumaSize : e.lumaSize+e.chromaSize])
	picIn.Img.Plane[2] = C.CBytes(yuv[e.lumaSize+e.chromaSize:])

	picIn.IPts = e.pts
	e.pts++

	defer func() {
		C.free(picIn.Img.Plane[0])
		C.free(picIn.Img.Plane[1])
		C.free(picIn.Img.Plane[2])
	}()

	if ret := x264.EncoderEncode(e.ref, e.nals, &e.nnals, &picIn, &picOut); ret > 0 {
		return C.GoBytes(e.nals[0].PPayload, C.int(ret))
		// ret should be equal to writer writes
	}
	return []byte{}
}

func (e *H264) Shutdown() error {
	x264.EncoderClose(e.ref)
	return nil
}
