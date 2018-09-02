package tmx

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// Data contains the tile data for a map
type Data struct {
	// Encoding is the encoding used for the data. It can either be "base64"
	// or "csv"
	Encoding string `xml:"encoding,attr"`
	// Compression is the compression used for the data. It can either be
	// "gzip" or "zlib"
	Compression string `xml:"compression,attr"`
	// Tiles are the tiles in the data. Not the same as TMXTiles from the Tileset.
	Tiles []TileData `xml:"tile"`
	// Chunks are sets of tiles over an area. Used for randomly generated maps.
	Chunks []Chunk `xml:"chunk"`
	// Inner is the inner data
	Inner string `xml:",innerxml"`
}

// Chunk contains chunk data for a map. A chunk is a set of more than one
// tile that goes together, so when the map is set to randomly generate, these
// tiles are generated together.
type Chunk struct {
	// X is the x coordinate of the chunk in tiles
	X int `xml:"x,attr"`
	// Y is the y coordinate of the chunk in tiles
	Y int `xml:"y,attr"`
	// Width is the width of the chunk in tiles
	Width int `xml:"width,attr"`
	// Height is the height of the chunk in tiles
	Height int `xml:"height,attr"`
	// Tiles are the tiles in the chunk
	Tiles []TileData `xml:"tile"`
}

// TileData contains the gid that maps a tile to the sprite
type TileData struct {
	// RawGID is the global tile ID given in the map
	RawGID uint32 `xml:"gid,attr"`
	// GID is the global tile ID with the flipping bits removed
	GID uint32
	// Flipping is the flipping flags present
	// You can & this with the constants HorizontalFlipFlag, VerticalFlipFlag, and
	// DiagonalFlipFlag to find out if the flag was present on the tile.
	Flipping uint32
}

const (
	// HorizontalFlipFlag is a flag for a horizontally flipped tile
	HorizontalFlipFlag uint32 = 0x80000000
	// VerticalFlipFlag is a flag for a vertically flipped tile
	VerticalFlipFlag uint32 = 0x40000000
	// DiagonalFlipFlag is a flag for a diagonally flipped tile
	DiagonalFlipFlag uint32 = 0x20000000
)

// UnmarshalXML implements the encoding/xml Unmarshaler interface
func (da *Data) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type data Data
	dat := data{}
	if err := d.DecodeElement(&dat, &start); err != nil {
		return err
	}
	*da = (Data)(dat)
	if len(da.Tiles) > 0 {
		return nil
	}
	if da.Encoding == "csv" {
		b := strings.NewReader(strings.TrimSpace(da.Inner))
		cr := csv.NewReader(b)
		// We allow variable number of fields per record to allow line ending commas and then
		// empty strings appearing as a field. Later, we filter empty strings. This trick is
		// needed to allow TilEd-style CSVs with line-ending commas but no comma at the end
		// of last line.
		cr.FieldsPerRecord = -1
		if recs, err := cr.ReadAll(); err == nil {
			if len(recs) < 1 {
				return errors.New("No csv records found")
			}
			for _, rec := range recs {
				for i, id := range rec {
					// An empty string appearing after last comma. We filter it.
					if id == "" && i == len(rec)-1 {
						continue
					}
					if nextInt, err2 := strconv.ParseUint(id, 10, 32); err == nil {
						da.Tiles = append(da.Tiles, TileData{GID: uint32(nextInt)})
					} else {
						return err2
					}
				}
			}
			if len(da.Tiles) < 1 {
				return errors.New("No Data Returned")
			}
		} else {
			return err
		}
		return nil
	}
	var breader io.Reader
	if da.Encoding == "base64" {
		buff, err := base64.StdEncoding.DecodeString(strings.TrimSpace(da.Inner))
		if err != nil {
			return err
		}
		breader = bytes.NewReader(buff)
	} else {
		return errors.New("Unknown Encoding")
	}
	// Setup decompression if needed
	var zreader io.Reader
	if da.Compression == "" {
		zreader = breader
	} else if da.Compression == "zlib" {
		z, err := zlib.NewReader(breader)
		if err != nil {
			return err
		}
		defer z.Close()
		zreader = z
	} else if da.Compression == "gzip" {
		z, err := gzip.NewReader(breader)
		if err != nil {
			return err
		}
		defer z.Close()
		zreader = z
	} else {
		return errors.New("Unknown Compression")
	}
	var nextInt uint32
	for {
		err := binary.Read(zreader, binary.LittleEndian, &nextInt)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		g, f := decodeGID(nextInt)
		da.Tiles = append(da.Tiles, TileData{
			RawGID:   nextInt,
			GID:      g,
			Flipping: f,
		})
	}
	return nil
}

func decodeGID(u uint32) (uint32, uint32) {
	h := u & HorizontalFlipFlag
	v := u & VerticalFlipFlag
	d := u & DiagonalFlipFlag
	ret := u & ^(HorizontalFlipFlag | VerticalFlipFlag | DiagonalFlipFlag)
	return ret, h | v | d
}