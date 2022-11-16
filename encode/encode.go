/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/16 2:07 PM
 */
package encode

import msgpack "github.com/wasmcloud/tinygo-msgpack"

type Encoder interface {
	MEncode(encoder msgpack.Writer) error
}

func Encode(e Encoder) ([]byte, error) {
	// Encode response to actor
	var sizer msgpack.Sizer
	sizeEnc := &sizer
	err := e.MEncode(sizeEnc)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, sizer.Len())
	encoder := msgpack.NewEncoder(buf)
	enc := &encoder
	err = e.MEncode(enc)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
