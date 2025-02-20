package riff

// TODO:
// mp3 mappig, seem there can be many frames in one sample and they span samples?
// hevc mapping?
// DV handler https://learn.microsoft.com/en-us/windows/win32/directshow/dv-data-in-the-avi-file-format
// palette change
// rec groups
// AVIX, multiple RIFF headers?
// nested indexes
// unknown fields for unreachable chunk header for > 1gb samples
// 2fields, field index?

// https://learn.microsoft.com/en-us/windows/win32/directshow/avi-riff-file-reference
// http://www.jmcgowan.com/odmlff2.pdf
// https://github.com/FFmpeg/FFmpeg/blob/master/libavformat/avidec.c
// https://github.com/tpn/winsdk-10/blob/master/Include/10.0.16299.0/um/aviriff.h

import (
	"embed"
	"strconv"

	"github.com/wader/fq/format"
	"github.com/wader/fq/pkg/decode"
	"github.com/wader/fq/pkg/interp"
	"github.com/wader/fq/pkg/ranges"
	"github.com/wader/fq/pkg/scalar"
)

//go:embed avi.md
var aviFS embed.FS

var aviMp3FrameGroup decode.Group
var aviMpegAVCAUGroup decode.Group
var aviMpegHEVCAUGroup decode.Group
var aviFLACFrameGroup decode.Group

func init() {
	interp.RegisterFormat(
		format.AVI,
		&decode.Format{
			Description: "Audio Video Interleaved",
			DecodeFn:    aviDecode,
			DefaultInArg: format.AVI_In{
				DecodeSamples: true,
			},
			Dependencies: []decode.Dependency{
				{Groups: []*decode.Group{format.AVC_AU}, Out: &aviMpegAVCAUGroup},
				{Groups: []*decode.Group{format.HEVC_AU}, Out: &aviMpegHEVCAUGroup},
				{Groups: []*decode.Group{format.MP3_Frame}, Out: &aviMp3FrameGroup},
				{Groups: []*decode.Group{format.FLAC_Frame}, Out: &aviFLACFrameGroup},
			},
			Groups: []*decode.Group{format.Probe},
		})
	interp.RegisterFS(aviFS)
}

var aviListTypeDescriptions = scalar.StrMapDescription{
	"hdrl": "AVI main list",
	"strl": "Stream list",
	"movi": "Stream Data",
	"rec ": "Chunk group",
}

var aviStrhTypeDescriptions = scalar.StrMapDescription{
	"auds": "Audio stream",
	"mids": "MIDI stream",
	"txts": "Text stream",
	"vids": "Video stream",
}

const (
	aviIndexTypeIndexes = 0
	aviIndexTypeChunks  = 1
)

var aviIndexTypeNames = scalar.UintMapSymStr{
	aviIndexTypeIndexes: "indexes",
	aviIndexTypeChunks:  "chunks",
}

const (
	aviIndexSubType2Fields = 1
)

var aviIndexSubTypeNames = scalar.UintMapSymStr{
	aviIndexSubType2Fields: "2fields",
}

const (
	aviStreamChunkTypeUncompressedVideo = "db"
	aviStreamChunkTypeCompressedVideo   = "dc"
	aviStreamChunkTypePaletteChange     = "pc"
	aviStreamChunkTypeAudio             = "wb"
	aviStreamChunkTypeIndex             = "ix"
)

var aviStreamChunkTypeDescriptions = scalar.StrMapDescription{
	aviStreamChunkTypeUncompressedVideo: "Uncompressed video frame",
	aviStreamChunkTypeCompressedVideo:   "Compressed video frame",
	aviStreamChunkTypePaletteChange:     "Palette change",
	aviStreamChunkTypeAudio:             "Audio data",
	aviStreamChunkTypeIndex:             "Index",
}

const aviRiffType = "AVI "

type aviStrl struct {
	typ     string
	handler string
	stream  *aviStream
}

type idx1Sample struct {
	offset     int64
	size       int64
	streamNr   int
	streamType string
}

type aviStream struct {
	hasFormat   bool
	format      decode.Group
	formatInArg any
	indexes     []ranges.Range
	ixSamples   []ranges.Range
}

func aviParseChunkID(id string) (string, int, bool) {
	if len(id) != 4 {
		return "", 0, false
	}

	isDigits := func(s string) bool {
		for _, c := range s {
			if !(c >= '0' && c <= '9') {
				return false
			}
		}
		return true
	}

	var typ string
	var indexStr string
	switch {
	case isDigits(id[0:2]):
		// ##dc media etc
		indexStr, typ = id[0:2], id[2:4]
	case isDigits(id[2:4]):
		// ix## index etc
		typ, indexStr = id[0:2], id[2:4]
	default:
		return "", 0, false
	}

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		panic("unreachable")
	}

	return typ, index, true

}

func aviIsStreamType(typ string) bool {
	switch typ {
	case aviStreamChunkTypeUncompressedVideo,
		aviStreamChunkTypeCompressedVideo,
		aviStreamChunkTypeAudio:
		return true
	default:
		return false
	}
}

func aviDecorateStreamID(d *decode.D, id string) (string, int) {
	typ, index, ok := aviParseChunkID(id)
	if ok && aviIsStreamType(typ) {
		d.FieldValueStr("stream_type", typ, aviStreamChunkTypeDescriptions)
		d.FieldValueUint("stream_nr", uint64(index))
		return typ, index
	}
	return "", 0
}

// ix frame index and indx frame index
func aviDecodeChunkIndex(d *decode.D) []ranges.Range {
	var rs []ranges.Range

	d.FieldU16("longs_per_entry") // TODO: use?
	d.FieldU8("index_subtype", aviIndexSubTypeNames)
	d.FieldU8("index_type", aviIndexTypeNames)
	nEntriesInUse := d.FieldU32("entries_in_use")
	chunkID := d.FieldUTF8("chunk_id", 4)
	aviDecorateStreamID(d, chunkID)
	baseOffset := int64(d.FieldU64("base_offset"))
	d.FieldU32("unused")
	d.FieldArray("index", func(d *decode.D) {
		for i := 0; i < int(nEntriesInUse); i++ {
			d.FieldStruct("index", func(d *decode.D) {
				offset := int64(d.FieldU32("offset"))
				sizeKeyFrame := d.FieldU32("size_keyframe")
				size := sizeKeyFrame & 0x7f_ff_ff_ff
				d.FieldValueUint("size", size)
				d.FieldValueBool("key_frame", sizeKeyFrame&0x80_00_00_00 == 0)
				rs = append(rs, ranges.Range{
					Start: baseOffset*8 + offset*8,
					Len:   int64(size) * 8,
				})
			})
		}
	})

	return rs
}

func aviDecode(d *decode.D) any {
	var ai format.AVI_In
	d.ArgAs(&ai)

	d.Endian = decode.LittleEndian

	var streams []*aviStream
	var idx1Samples []idx1Sample
	var moviListPos int64 // point to first bit after type

	var riffType string
	riffDecode(
		d,
		nil,
		func(d *decode.D, path path) (string, int64) {
			id := d.FieldUTF8("id", 4, chunkIDDescriptions)
			aviDecorateStreamID(d, id)
			size := d.FieldU32("size")
			return id, int64(size)
		},
		func(d *decode.D, id string, path path) (bool, any) {
			switch id {
			case "RIFF":
				riffType = d.FieldUTF8("type", 4, d.StrAssert(aviRiffType))
				return true, nil

			case "LIST":
				typ := d.FieldUTF8("type", 4, aviListTypeDescriptions)
				switch typ {
				case "strl":
					return true, &aviStrl{}
				case "movi":
					moviListPos = d.Pos()
				}
				return true, nil

			case "idx1":
				d.FieldArray("indexes", func(d *decode.D) {
					// TODO: seems there are files with weird tailing extra index entries
					// TODO: limit using total_frame somehow instead?
					for d.BitsLeft() >= 4*32 {
						d.FieldStruct("index", func(d *decode.D) {
							id := d.FieldUTF8("id", 4)
							typ, index := aviDecorateStreamID(d, id)
							d.FieldStruct("flags", func(d *decode.D) {
								d.FieldRawLen("unused0", 3)
								d.FieldBool("key_frame")
								d.FieldRawLen("unused1", 3)
								d.FieldBool("list")
								d.FieldRawLen("unused2", 24)
							})
							offset := int64(d.FieldU32("offset"))
							length := int64(d.FieldU32("length"))

							idx1Samples = append(idx1Samples, idx1Sample{
								offset:     offset * 8,
								size:       length * 8,
								streamNr:   index,
								streamType: typ,
							})
						})
					}
				})
				return false, nil

			case "avih":
				d.FieldU32("micro_sec_per_frame")
				d.FieldU32("max_bytes_per_sec")
				d.FieldU32("padding_granularity")
				d.FieldStruct("flags", func(d *decode.D) {
					d.FieldRawLen("unused0", 2)
					d.FieldBool("must_use_index")
					d.FieldBool("has_index") // Index at end of file?
					d.FieldRawLen("unused1", 8)
					d.FieldBool("trust_ck_type") // Use CKType to find key frames
					d.FieldRawLen("unused2", 2)
					d.FieldBool("is_interleaved")
					d.FieldRawLen("unused3", 6)
					d.FieldBool("copyrighted")
					d.FieldBool("was_capture_file")
					d.FieldRawLen("unused4", 8)
				})
				d.FieldU32("total_frames")
				d.FieldU32("initial_frames")
				d.FieldU32("streams")
				d.FieldU32("suggested_buffer_size")
				d.FieldU32("width")
				d.FieldU32("height")
				d.FieldRawLen("reserved", 32*4)
				return false, nil

			case "dmlh":
				d.FieldU32("total_frames")
				d.FieldRawLen("future", 32*61)
				return false, nil

			case "strh":
				typ := d.FieldUTF8("type", 4, aviStrhTypeDescriptions)
				handler := d.FieldUTF8("handler", 4)
				d.FieldStruct("flags", func(d *decode.D) {
					d.FieldRawLen("unused0", 7)
					d.FieldBool("disabled")
					d.FieldRawLen("unused1", 15)
					d.FieldBool("pal_changes")
					d.FieldRawLen("unused2", 8)
				})
				d.FieldU16("priority")
				d.FieldU16("language")
				d.FieldU32("initial_frames")
				d.FieldU32("scale")
				d.FieldU32("rate")
				d.FieldU32("start")
				d.FieldU32("length")
				d.FieldU32("suggested_buffer_size")
				d.FieldU32("quality")
				d.FieldU32("sample_size")
				d.FieldStruct("frame", func(d *decode.D) {
					d.FieldU16("left")
					d.FieldU16("top")
					d.FieldU16("right")
					d.FieldU16("bottom")
				})

				if aviStrl, aviStrlOk := path.topData().(*aviStrl); aviStrlOk {
					aviStrl.typ = typ
					aviStrl.handler = handler
				}

				return false, nil

			case "strf":
				s := &aviStream{}

				typ := ""
				if aviStrl, aviStrlOk := path.topData().(*aviStrl); aviStrlOk {
					typ = aviStrl.typ
					aviStrl.stream = s
				}

				switch typ {
				case "vids":
					// BITMAPINFOHEADER
					size := d.BitsLeft()
					biSize := d.FieldU32("bi_size")
					d.FieldU32("width")
					d.FieldU32("height")
					d.FieldU16("planes")
					d.FieldU16("bit_count")
					compression := d.FieldUTF8("compression", 4)
					d.FieldU32("size_image")
					d.FieldU32("x_pels_per_meter")
					d.FieldU32("y_pels_per_meter")
					d.FieldU32("clr_used")
					d.FieldU32("clr_important")
					extraSize := size - int64(biSize)*8 - 2*32
					if extraSize > 0 {
						d.FieldRawLen("extra", extraSize)
					}

					// TODO: if dvsd handler and extraSize >= 32 then DVINFO?

					switch compression {
					case format.BMPTagH264,
						format.BMPTagH264_h264,
						format.BMPTagH264_X264,
						format.BMPTagH264_x264,
						format.BMPTagH264_avc1,
						format.BMPTagH264_DAVC,
						format.BMPTagH264_SMV2,
						format.BMPTagH264_VSSH,
						format.BMPTagH264_Q264,
						format.BMPTagH264_V264,
						format.BMPTagH264_GAVC,
						format.BMPTagH264_UMSV,
						format.BMPTagH264_tshd,
						format.BMPTagH264_INMC:
						s.format = aviMpegAVCAUGroup
						s.hasFormat = true
					case format.BMPTagHEVC,
						format.BMPTagHEVC_H265:
						s.format = aviMpegHEVCAUGroup
						s.hasFormat = true
					}

				case "auds":
					// WAVEFORMATEX
					formatTag := d.FieldU16("format_tag", format.WAVTagNames)
					d.FieldU16("channels")
					d.FieldU32("samples_per_sec")
					d.FieldU32("avg_bytes_per_sec")
					d.FieldU16("block_align")
					d.FieldU16("bits_per_sample")
					// TODO: seems to be optional
					if d.BitsLeft() >= 16 {
						cbSize := d.FieldU16("cb_size")
						// TODO: correct? seems to be what is seen in the wild
						if cbSize > 18 {
							d.FieldRawLen("extra", int64(cbSize-18)*8)
						}
					}

					switch formatTag {
					case format.WAVTagMP3:
						s.format = aviMp3FrameGroup
						s.hasFormat = true
					case format.WAVTagFLAC:
						// TODO: can flac in avi have streaminfo somehow?
						s.format = aviFLACFrameGroup
						s.hasFormat = true
					}
				case "iavs":
					// DVINFO
					d.FieldU32("dva_aux_src")
					d.FieldU32("dva_aux_ctl")
					d.FieldU32("dva_aux_src1")
					d.FieldU32("dva_aux_ctl1")
					d.FieldU32("dvv_aux_src")
					d.FieldU32("dvv_aux_ctl")
					d.FieldRawLen("dvv_reserved", 32*2)
				}

				streams = append(streams, s)

				return false, nil

			case "indx":
				var stream *aviStream
				if aviStrl, aviStrlOk := path.topData().(*aviStrl); aviStrlOk {
					stream = aviStrl.stream
				}

				d.FieldU16("longs_per_entry") // TODO: use?
				d.FieldU8("index_subtype")
				d.FieldU8("index_type")
				nEntriesInUse := d.FieldU32("entries_in_use")
				chunkID := d.FieldUTF8("chunk_id", 4)
				aviDecorateStreamID(d, chunkID)
				d.FieldU64("base")
				d.FieldU32("unused")
				d.FieldArray("index", func(d *decode.D) {
					for i := 0; i < int(nEntriesInUse); i++ {
						d.FieldStruct("index", func(d *decode.D) {
							offset := int64(d.FieldU64("offset"))
							size := int64(d.FieldU32("size"))
							d.FieldU32("duration")

							if stream != nil {
								stream.indexes = append(stream.indexes, ranges.Range{
									Start: offset * 8,
									Len:   size * 8,
								})
							}
						})
					}
				})

				return false, nil

			case "vprp":
				d.FieldU32("video_format_token")
				d.FieldU32("video_standard")
				d.FieldU32("vertical_refresh_rate")
				d.FieldU32("h_total_in_t")
				d.FieldU32("v_total_in_lines")
				d.FieldStruct("frame_aspect_ratio", func(d *decode.D) {
					d.FieldU16("x")
					d.FieldU16("y")
				})
				d.FieldU32("frame_width_in_pixels")
				d.FieldU32("frame_height_in_lines")
				nbFieldPerFrame := d.FieldU32("nb_field_per_frame")
				d.FieldArray("field_info", func(d *decode.D) {
					for i := 0; i < int(nbFieldPerFrame); i++ {
						d.FieldStruct("field_info", func(d *decode.D) {
							d.FieldU32("compressed_bm_height")
							d.FieldU32("compressed_bm_width")
							d.FieldU32("valid_bm_height")
							d.FieldU32("valid_bm_width")
							d.FieldU32("valid_bmx_offset")
							d.FieldU32("valid_bmy_offset")
							d.FieldU32("video_x_offset_in_t")
							d.FieldU32("video_y_valid_start_line")
						})
					}
				})
				return false, nil

			default:
				if riffIsStringChunkID(id) {
					d.FieldUTF8NullFixedLen("value", int(d.BitsLeft())/8)
					return false, nil
				}

				typ, index, _ := aviParseChunkID(id)
				switch {
				case typ == "ix":
					sampleRanges := aviDecodeChunkIndex(d)
					if index < len(streams) {
						s := streams[index]
						s.ixSamples = append(s.ixSamples, sampleRanges...)
					}
				case d.BitsLeft() > 0 &&
					ai.DecodeSamples &&
					aviIsStreamType(typ) &&
					index < len(streams) &&
					streams[index].hasFormat:
					s := streams[index]
					d.FieldFormatLen("data", d.BitsLeft(), &s.format, s.formatInArg)
				default:
					d.FieldRawLen("data", d.BitsLeft())
				}

				return false, nil
			}
		},
	)

	if riffType != aviRiffType {
		d.Errorf("wrong or no AVI riff type found (%s)", riffType)
	}

	d.FieldArray("streams", func(d *decode.D) {
		for si, s := range streams {
			d.FieldStruct("stream", func(d *decode.D) {
				var streamIndexSampleRanges []ranges.Range
				if len(s.indexes) > 0 {
					d.FieldArray("indexes", func(d *decode.D) {
						for _, i := range s.indexes {
							d.FieldStruct("index", func(d *decode.D) {
								d.RangeFn(i.Start, i.Len, func(d *decode.D) {
									d.FieldUTF8("type", 4)
									d.FieldU32("cb")
									sampleRanges := aviDecodeChunkIndex(d)
									streamIndexSampleRanges = append(streamIndexSampleRanges, sampleRanges...)
								})
							})
						}
					})
				}

				// TODO: handle zero length samples differently?
				// TODO: palette change
				decodeSample := func(d *decode.D, sr ranges.Range) {
					d.RangeFn(sr.Start, sr.Len, func(d *decode.D) {
						if sr.Len > 0 && ai.DecodeSamples && s.hasFormat {
							d.FieldFormat("sample", &s.format, s.formatInArg)
						} else {
							d.FieldRawLen("sample", d.BitsLeft())
						}
					})
				}

				// try only add indexed samples once with priority:
				// stream index
				// ix chunks (might be same as stream index)
				// idx chunks
				if len(streamIndexSampleRanges) > 0 {
					d.FieldArray("samples", func(d *decode.D) {
						for _, sr := range streamIndexSampleRanges {
							decodeSample(d, sr)
						}
					})
				} else if len(s.ixSamples) > 0 {
					d.FieldArray("samples", func(d *decode.D) {
						for _, sr := range s.ixSamples {
							decodeSample(d, sr)
						}
					})
				} else if len(idx1Samples) > 0 {
					d.FieldArray("samples", func(d *decode.D) {
						for _, is := range idx1Samples {
							if is.streamNr != si {
								continue
							}
							decodeSample(d, ranges.Range{
								Start: moviListPos + is.offset + 32, // +32 skip size field
								Len:   is.size,
							})
						}
					})
				}
			})
		}
	})

	return nil
}
