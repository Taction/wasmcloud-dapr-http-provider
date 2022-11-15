/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/14 11:16 AM
 */
package log

import (
	"os"

	"github.com/sirupsen/logrus"
)

var Log = logrus.New()

func init() {
	os.Setenv("RUST_LOG", "debug")

	file, err := os.OpenFile("/tmp/httpprovider_go.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		Log.Out = file
	} else {
		Log.Info("Failed to log to file, using default stderr")
	}

	Log.SetFormatter(&logrus.JSONFormatter{})
	Log.SetReportCaller(true)
}
