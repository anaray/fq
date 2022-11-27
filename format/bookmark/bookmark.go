package bplist

import (
	"embed"
	"fmt"
	"time"

	"github.com/wader/fq/format"
	"github.com/wader/fq/pkg/decode"
	"github.com/wader/fq/pkg/interp"
	"github.com/wader/fq/pkg/scalar"
)

//go:embed bookmark.jq bookmark.md
var bookmarkFS embed.FS

func init() {
	interp.RegisterFormat(decode.Format{
		Name:        format.BOOKMARK,
		ProbeOrder:  format.ProbeOrderBinUnique,
		Description: "Apple BookmarkData",
		Groups:      []string{format.PROBE},
		DecodeFn:    bookmarkDecode,
		Functions:   []string{"torepr"},
	})
	interp.RegisterFS(bookmarkFS)
}

const (
	dataTypeString       = 0x0101
	dataTypeData         = 0x0201
	dataTypeNumber8      = 0x0301
	dataTypeNumber16     = 0x0302
	dataTypeNumber32     = 0x0303
	dataTypeNumber64     = 0x0304
	dataTypeNumber32F    = 0x0305
	dataTypeNumber64F    = 0x0306
	dataTypeDate         = 0x0400
	dataTypeBooleanFalse = 0x0500
	dataTypeBooleanTrue  = 0x0501
	dataTypeArray        = 0x0601
	dataTypeDictionary   = 0x0701
	dataTypeUUID         = 0x0801
	dataTypeURL          = 0x0901
	dataTypeRelativeURL  = 0x0902
)

var dataTypeMap = scalar.UToScalar{
	dataTypeString:       {Sym: "string", Description: "UTF-8 String"},
	dataTypeData:         {Sym: "data", Description: "Raw bytes"},
	dataTypeNumber8:      {Sym: "byte", Description: "(signed 8-bit) 1-byte number"},
	dataTypeNumber16:     {Sym: "short", Description: "(signed 16-bit) 2-byte number"},
	dataTypeNumber32:     {Sym: "int", Description: "(signed 32-bit) 4-byte number"},
	dataTypeNumber64:     {Sym: "long", Description: "(signed 64-bit) 8-byte number"},
	dataTypeNumber32F:    {Sym: "float", Description: "(32-bit float) IEEE single precision"},
	dataTypeNumber64F:    {Sym: "double", Description: "(64-bit float) IEEE double precision"},
	dataTypeDate:         {Sym: "date", Description: "Big-endian IEEE double precision seconds since 2001-01-01 00:00:00 UTC"},
	dataTypeBooleanFalse: {Sym: "boolean_false", Description: "False"},
	dataTypeBooleanTrue:  {Sym: "boolean_true", Description: "True"},
	dataTypeArray:        {Sym: "array", Description: "Array of 4-byte offsets to data items"},
	dataTypeDictionary:   {Sym: "dictionary", Description: "Array of pairs of 4-byte (key, value) data item offsets"},
	dataTypeUUID:         {Sym: "uuid", Description: "Raw bytes"},
	dataTypeURL:          {Sym: "url", Description: "UTF-8 string"},
	dataTypeRelativeURL:  {Sym: "relative_url", Description: "4-byte offset to base URL, 4-byte offset to UTF-8 string"},
}

const (
	elementTypeTargetURL             = 0x1003
	elementTypeTargetPath            = 0x1004
	elementTypeTargetCNIDPath        = 0x1005
	elementTypeTargetFlags           = 0x1010
	elementTypeTargetFilename        = 0x1020
	elementTypeCNID                  = 0x1030
	elementTypeTargetCreationDate    = 0x1040
	elementTypeUnknown1              = 0x1054
	elementTypeUnknown2              = 0x1055
	elementTypeUnknown3              = 0x1056
	elementTypeUnknown4              = 0x1101
	elementTypeUnknown5              = 0x1102
	elementTypeTOCPath               = 0x2000
	elementTypeVolumePath            = 0x2002
	elementTypeVolumeURL             = 0x2005
	elementTypeVolumeName            = 0x2010
	elementTypeVolumeUUID            = 0x2011
	elementTypeVolumeSize            = 0x2012
	elementTypeVolumeCreationDate    = 0x2013
	elementTypeVolumeFlags           = 0x2020
	elementTypeVolumeIsRoot          = 0x2030
	elementTypeVolumeBookmark        = 0x2040
	elementTypeVolumeMountPointURL   = 0x2050
	elementTypeUnknown6              = 0x2070
	elementTypeContainingFolderIndex = 0xc001
	elementTypeCreatorUsername       = 0xc011
	elementTypeCreatorUID            = 0xc012
	elementTypeFileReferenceFlag     = 0xd001
	elementTypeCreationOptions       = 0xd010
	elementTypeURLLengthArray        = 0xe003
	elementTypeDisplayName           = 0xf017
	elementTypeIconData              = 0xf020
	elementTypeIconImageData         = 0xf021
	elementTypeTypeBindingInfo       = 0xf022
	elementTypeBookmarkCreationTime  = 0xf030
	elementTypeSandboxRWExtension    = 0xf080
	elementTypeSandboxROExtension    = 0xf081
)

var elementTypeMap = scalar.UToScalar{
	elementTypeTargetURL:             {Sym: "target_url", Description: "A URL"},
	elementTypeTargetPath:            {Sym: "target_path", Description: "Array of individual path components"},
	elementTypeTargetCNIDPath:        {Sym: "target_cnid_path", Description: "Array of CNIDs"},
	elementTypeTargetFlags:           {Sym: "target_flags", Description: "Data - see below"},
	elementTypeTargetFilename:        {Sym: "target_filename", Description: "String"},
	elementTypeCNID:                  {Sym: "target_cnid", Description: "4-byte integer"},
	elementTypeTargetCreationDate:    {Sym: "target_creation_date", Description: "Date"},
	elementTypeUnknown1:              {Sym: "unknown", Description: "Unknown"},
	elementTypeUnknown2:              {Sym: "unknown", Description: "Unknown"},
	elementTypeUnknown3:              {Sym: "unknown", Description: "Unknown"},
	elementTypeUnknown4:              {Sym: "unknown", Description: "Unknown"},
	elementTypeUnknown5:              {Sym: "unknown", Description: "Unknown"},
	elementTypeTOCPath:               {Sym: "toc_path", Description: "Array - see below"},
	elementTypeVolumePath:            {Sym: "volume_path", Description: "Array of individual path components"},
	elementTypeVolumeURL:             {Sym: "volume_url", Description: "URL of volume root"},
	elementTypeVolumeName:            {Sym: "volume_name", Description: "String"},
	elementTypeVolumeUUID:            {Sym: "volume_uuid", Description: "String UUID"},
	elementTypeVolumeSize:            {Sym: "volume_size", Description: "8-byte integer"},
	elementTypeVolumeCreationDate:    {Sym: "volume_creation_date", Description: "Date"},
	elementTypeVolumeFlags:           {Sym: "volume_flags", Description: "Data - see below"},
	elementTypeVolumeIsRoot:          {Sym: "volume_is_root", Description: "True if the volume was the filesystem root"},
	elementTypeVolumeBookmark:        {Sym: "volume_bookmark", Description: "TOC identifier for disk image"},
	elementTypeVolumeMountPointURL:   {Sym: "volume_mount_point", Description: "URL"},
	elementTypeUnknown6:              {Sym: "unknown", Description: "Unknown"},
	elementTypeContainingFolderIndex: {Sym: "containing_folder_index", Description: "Integer index of containing folder in target path array"},
	elementTypeCreatorUsername:       {Sym: "creator_username", Description: "Name of user that created bookmark"},
	elementTypeCreatorUID:            {Sym: "creator_uid", Description: "UID of user that created bookmark"},
	elementTypeFileReferenceFlag:     {Sym: "file_reference_flag", Description: "True if creating URL was a file reference URL"},
	elementTypeCreationOptions:       {Sym: "creation_options", Description: "Integer containing flags passed to CFURLCreateBookmarkData"},
	elementTypeURLLengthArray:        {Sym: "url_length_array", Description: "Array of integers"},
	elementTypeDisplayName:           {Sym: "display_name", Description: "String"},
	elementTypeIconData:              {Sym: "icon_data", Description: "icns format data"},
	elementTypeIconImageData:         {Sym: "icon_image", Description: "Data"},
	elementTypeTypeBindingInfo:       {Sym: "type_binding_info", Description: "dnib byte array"},
	elementTypeBookmarkCreationTime:  {Sym: "bookmark_creation_time", Description: "64-bit float seconds since January 1st 2001"},
	elementTypeSandboxRWExtension:    {Sym: "sandbox_rw_extension", Description: "Looks like a hash with data and an access right"},
	elementTypeSandboxROExtension:    {Sym: "sandbox_ro_extension", Description: "Looks like a hash with data and an access right"},
}

var cocoaTimeEpochDate = time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC)

type tocHeader struct {
	tocSize          uint64
	nextTOCOffset    uint64
	numEntries       uint64
	entryArrayOffset int64
}

func decodeTOCHeader(d *decode.D, idx int) *tocHeader {
	hdr := new(tocHeader)

	d.FieldStruct(fmt.Sprintf("toc_header_%d", idx), func(d *decode.D) {
		hdr.tocSize = d.FieldU32("toc_size")
		d.FieldU32("magic", d.AssertU(0xfffffffe))
		d.FieldU32("identifier")
		hdr.nextTOCOffset = d.FieldU32("next_toc_offset")
		hdr.numEntries = d.FieldU32("num_entries_in_toc")
		hdr.entryArrayOffset = d.Pos()
	})

	return hdr
}

type tocEntry struct {
	key          uint64
	recordOffset int64
}

const (
	arrayEntrySize = 4
	dictEntrySize  = 4
)

func decodeRecord(d *decode.D) {
	d.FieldStruct("record", func(d *decode.D) {
		n := int(d.FieldU32("length"))
		typ := d.FieldU32("type", dataTypeMap)
		switch typ {
		case dataTypeString:
			d.FieldUTF8("data", n)
		case dataTypeData:
			d.FieldRawLen("data", int64(n*8))
		case dataTypeNumber8:
			d.FieldS8("data")
		case dataTypeNumber16:
			d.FieldS16("data")
		case dataTypeNumber32:
			d.FieldS32("data")
		case dataTypeNumber64:
			d.FieldS64("data")
		case dataTypeNumber32F:
			d.FieldF32("data")
		case dataTypeNumber64F:
			d.FieldF64("data")
		case dataTypeDate:
			d.FieldF64BE("data", scalar.DescriptionTimeFn(scalar.S.TryActualF, cocoaTimeEpochDate, time.RFC3339))
		case dataTypeBooleanFalse:
		case dataTypeBooleanTrue:
		case dataTypeArray:
			d.FieldStructNArray("data", "element", int64(n/arrayEntrySize), func(d *decode.D) {
				offset := calcOffset(d.FieldU32("offset"))
				d.SeekAbs(int64(offset), decodeRecord)
			})
		case dataTypeDictionary:
			d.FieldStructNArray("data", "element", int64(n/dictEntrySize), func(d *decode.D) {
				keyOffset := calcOffset(d.FieldU32("key_offset"))
				d.FieldStruct("key", func(d *decode.D) {
					d.SeekAbs(keyOffset, decodeRecord)
				})

				valueOffset := calcOffset(d.FieldU32("value_offset"))
				d.FieldStruct("value", func(d *decode.D) {
					d.SeekAbs(int64(valueOffset), decodeRecord)
				})
			})
		case dataTypeUUID:
			d.FieldRawLen("data", int64(n*8))
		case dataTypeURL:
			d.FieldUTF8("data", n)
		case dataTypeRelativeURL:
			baseOffset := d.FieldU32("base_url_offset")
			d.FieldStruct("base_url", func(d *decode.D) {
				d.SeekAbs(int64(baseOffset), decodeRecord)
			})

			suffixOffset := d.FieldU32("suffix_offset")
			d.FieldStruct("suffix", func(d *decode.D) {
				d.SeekAbs(int64(suffixOffset), decodeRecord)
			})
		}
	})
}

const reservedSize = 32
const headerEnd = 48
const headerEndBitPos = headerEnd * 8

// all offsets are calculated relative to the end of the bookmark header
func calcOffset(i uint64) int64 { return int64(8 * (i + headerEnd)) }

func bookmarkDecode(d *decode.D, _ any) any {

	// all fields are little-endian with the exception of the Date datatype.
	d.Endian = decode.LittleEndian

	// decode bookmarkdata header, one at the top of each "file",
	// although these may be nested inside of binary plists
	d.FieldStruct("header", func(d *decode.D) {
		d.FieldUTF8("magic", 4, d.AssertStr("book", "alis"))
		d.FieldU32("total_size")
		d.FieldU32("unknown")
		d.FieldU32("header_size", d.AssertU(48))
		d.FieldRawLen("reserved", reservedSize*8)
	})

	tocOffset := calcOffset(d.FieldU32("first_toc_offset"))

	var tocHeaders []*tocHeader

	for i := 0; tocOffset != headerEndBitPos; i++ {
		// seek to the TOC, and decode the header and entries
		// for this TOC instance. SeekAbs restores our offset each time.
		d.SeekAbs(tocOffset, func(d *decode.D) {

			tocHdr := decodeTOCHeader(d, i)
			// store the toc header. we're going to decode the entries in one
			// big array once we have decoded all toc's
			tocHeaders = append(tocHeaders, tocHdr)
			// save the next toc_offset value. 0 indicates that we have reached
			// the last TOC instance.
			tocOffset = calcOffset(tocHdr.nextTOCOffset)

		})

		j := 0

		// now that we've collected all toc headers, iterate through each one's
		// entries and decode associated records.
		d.FieldArrayLoop("bookmark_entries",
			func() bool { return j < len(tocHeaders) },
			func(d *decode.D) {

				tocHdr := tocHeaders[j]
				j++

				d.SeekAbs(tocHdr.entryArrayOffset, func(d *decode.D) {
					for k := uint64(0); k < tocHdr.numEntries; k++ {
						entry := new(tocEntry)

						d.FieldStruct("entry", func(d *decode.D) {
							// entry.key = d.FieldU32("key", elementTypeMap)
							entry.key = d.FieldU32("key", elementTypeMap)

							// if the key has the top bit set, then (key & 0x7fffffff)
							// gives the offset of a string record.
							if entry.key&0x80000000 != 0 {
								d.FieldStruct("key_string", func(d *decode.D) {
									d.SeekAbs(int64(calcOffset(entry.key&0x7fffffff)), decodeRecord)
								})
							}

							entry.recordOffset = calcOffset(d.FieldU32("offset_to_record"))

							d.FieldU32("reserved")

							d.SeekAbs(int64(entry.recordOffset), decodeRecord)
						})
					}
				})
			})
	}

	return nil
}
