package rotatefile_test

import (
	"testing"
	"time"

	"github.com/gookit/goutil/fsutil"
	"github.com/gookit/goutil/mathutil"
	"github.com/gookit/goutil/testutil/assert"
	"github.com/gookit/slog/rotatefile"
)

// https://github.com/gookit/slog/issues/138
// 日志按everyday自动滚动，文件名的日期对应的是前一天的日志 #138
func TestIssues_138(t *testing.T) {
	logfile := "testdata/rotate_day.log"

	mt := newMockTime("2023-11-16 23:59:58")
	w, err := rotatefile.NewWriterWith(rotatefile.WithDebugMode, func(c *rotatefile.Config) {
		c.TimeClock = mt
		// c.MaxSize = 128
		c.Filepath = logfile
		c.RotateTime = rotatefile.EveryDay
	})

	assert.NoErr(t, err)
	defer w.MustClose()

	for i := 0; i < 5; i++ {
		dt := mt.Datetime()
		_, err = w.WriteString(dt + " [INFO] this is a log message, idx=" + mathutil.String(i) + "\n")
		assert.NoErr(t, err)
		mt.Add(time.Second) // add one second
	}

	// Out: rotate_day.log, rotate_day.log.20231116
	files := fsutil.Glob(logfile + "*")
	assert.Len(t, files, 2)

	// check contents
	assert.True(t, fsutil.IsFile(logfile))
	s := fsutil.ReadString(logfile)
	assert.StrContains(t, s, "2023-11-17 00:00:01 [INFO]")
	assert.StrNoContains(t, s, "2023-11-16")

	oldFile := logfile + ".20231116"
	assert.True(t, fsutil.IsFile(oldFile))
	s = fsutil.ReadString(oldFile)
	assert.StrContains(t, s, "2023-11-16 23:59:59 [INFO]")
	assert.StrNoContains(t, s, "2023-11-17")
}
