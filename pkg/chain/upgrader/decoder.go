package upgrader

//func NewDecoder() *Decoder {
//	return &Decoder{}
//}
//
//type Decoder struct {
//}
//
//func (d Decoder) DecodeParams(code cid.Cid, method abi.MethodNum, params []byte) (r []byte, methodName string, err error) {
//
//	registry := filcns.NewActorRegistry()
//
//	methodMeta, found := registry.Methods[code][method]
//	if !found {
//		err = fmt.Errorf("method %d not found on actor %s", method, code)
//		return
//	}
//
//	methodName = methodMeta.Name
//	p := reflect.New(methodMeta.Params.Elem()).Interface().(cbg.CBORUnmarshaler)
//	if err = p.UnmarshalCBOR(bytes.NewReader(params)); err != nil {
//		return
//	}
//
//	r, err = json.Marshal(p)
//	if err != nil {
//		return
//	}
//
//	return
//}
//
//func (d Decoder) DecodeReturn(code cid.Cid, method abi.MethodNum, ret []byte) (r []byte, err error) {
//	methodMeta, found := filcns.NewActorRegistry().Methods[code][method] // TODO: use remote
//	if !found {
//		err = fmt.Errorf("method %d not found on actor %s", method, code)
//		return
//	}
//	re := reflect.New(methodMeta.Ret.Elem())
//	p := re.Interface().(cbg.CBORUnmarshaler)
//	if err = p.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
//		return
//	}
//
//	r, err = json.Marshal(p)
//	if err != nil {
//		return
//	}
//	return
//}
