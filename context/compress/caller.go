// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package compress

import (
	"context"
	"errors"
)

// EngineCaller adapts a CompressModelChain to contextmgr.CompressEngine so
// the context manager's Reorganize drives compression through the same model
// chain (small → main → mechanical fallback) used by the compression engine.
type EngineCaller struct {
	chain *CompressModelChain
}

// NewEngineCaller builds a CompressEngine adapter over chain.
func NewEngineCaller(chain *CompressModelChain) *EngineCaller {
	return &EngineCaller{chain: chain}
}

// Call runs one L1 compression of prompt.
func (c *EngineCaller) Call(ctx context.Context, prompt string) (string, error) {
	if c.chain == nil {
		return "", errors.New("compress: caller has no model chain")
	}
	return c.chain.Compress(ctx, 1, prompt)
}
