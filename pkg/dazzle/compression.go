// Copyright © 2022 Gitpod

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package dazzle

import ociv1 "github.com/opencontainers/image-spec/specs-go/v1"

type Compression int

const (
	Gzip Compression = iota
	Zstd
)

func (compression *Compression) Extension() string {
	switch *compression {
	case Gzip:
		return ociv1.MediaTypeImageLayerGzip
	case Zstd:
		return ociv1.MediaTypeImageLayerZstd
	}
	return ociv1.MediaTypeImageLayerGzip
}

func (compression *Compression) String() string {
	switch *compression {
	case Gzip:
		return "gzip"
	case Zstd:
		return "zstd"
	}
	return "gzip"
}
