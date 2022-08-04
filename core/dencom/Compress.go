package dencom

import (
	"bytes"
	"compress/gzip"
	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	"io/ioutil"
)

//GZip压缩
func Compress(data []byte)([]byte,error){
	//压缩
	if len(data) == 0 {
		return data, nil
	}
	var b bytes.Buffer
	gzipWriter := gzip.NewWriter(&b)
	defer gzipWriter.Close()
	_,err := gzipWriter.Write(data)
	if err != nil{
		glog.V(logger.Info).Infoln("gzip write err :",err)
		return nil, err
	}
	err = gzipWriter.Flush()//刷新
	if err != nil{
		glog.V(logger.Info).Infoln("gzip flush err :",err)
		return nil, err
	}
	return b.Bytes(), nil
}

//GZip解压
func DeCompress(data []byte)([]byte,error){
	//解压缩
	if len(data) == 0 {
		return data, nil
	}
	var r bytes.Buffer
	r.Write(data)
	gzipReader, err := gzip.NewReader(&r)
	if err != nil{	//如果传入数据不是压缩过的，从这一步报错
		return nil, err
	}
	defer gzipReader.Close()
	dedata, _:= ioutil.ReadAll(gzipReader)

	return dedata, nil
}