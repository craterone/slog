package handler

import (
	"io"
	"io/fs"

	"github.com/craterone/slog"
	"github.com/craterone/slog/bufwrite"
	"github.com/craterone/slog/rotatefile"
	"github.com/gookit/goutil/errorx"
	"github.com/gookit/goutil/fsutil"
)

// the buff mode consts
const (
	BuffModeLine = "line"
	BuffModeBite = "bite"
)

const (
	// LevelModeList use level list for limit record write
	LevelModeList = slog.LevelModeList
	// LevelModeValue use max level limit log record write
	LevelModeValue = slog.LevelModeMax
)

// ConfigFn for config some settings
type ConfigFn func(c *Config)

// Config struct
type Config struct {
	// Logfile for write logs
	Logfile string `json:"logfile" yaml:"logfile"`

	// FilePerm for create log file. default rotatefile.DefaultFilePerm
	FilePerm fs.FileMode `json:"file_perm" yaml:"file_perm"`

	// LevelMode for limit log records. default LevelModeList
	LevelMode slog.LevelMode `json:"level_mode" yaml:"level_mode"`

	// Level max value. valid on LevelMode = LevelModeValue
	Level slog.Level `json:"level" yaml:"level"`

	// Levels list for write. valid on LevelMode = LevelModeList
	Levels []slog.Level `json:"levels" yaml:"levels"`

	// UseJSON for format logs
	UseJSON bool `json:"use_json" yaml:"use_json"`

	// BuffMode type name. allow: line, bite
	BuffMode string `json:"buff_mode" yaml:"buff_mode"`

	// BuffSize for enable buffer, unit is bytes. set 0 to disable buffer
	BuffSize int `json:"buff_size" yaml:"buff_size"`

	// RotateTime for rotate file, unit is seconds.
	RotateTime rotatefile.RotateTime `json:"rotate_time" yaml:"rotate_time"`

	// RotateMode for rotate file by time. default rotatefile.ModeRename
	RotateMode rotatefile.RotateMode `json:"rotate_mode" yaml:"rotate_mode"`

	// MaxSize on rotate file by size, unit is bytes.
	MaxSize uint64 `json:"max_size" yaml:"max_size"`

	// Compress determines if the rotated log files should be compressed using gzip.
	// The default is not to perform compression.
	Compress bool `json:"compress" yaml:"compress"`

	// BackupNum max number for keep old files.
	//
	// 0 is not limit, default is 20.
	BackupNum uint `json:"backup_num" yaml:"backup_num"`

	// BackupTime max time for keep old files, unit is hours.
	//
	// 0 is not limit, default is a week.
	BackupTime uint `json:"backup_time" yaml:"backup_time"`

	// RenameFunc build filename for rotate file
	RenameFunc func(filepath string, rotateNum uint) string

	// DebugMode for debug on development.
	DebugMode bool
}

// NewEmptyConfig new config instance
func NewEmptyConfig(fns ...ConfigFn) *Config {
	c := &Config{Levels: slog.AllLevels}
	return c.WithConfigFn(fns...)
}

// NewConfig new config instance with some default settings.
func NewConfig(fns ...ConfigFn) *Config {
	c := &Config{
		Levels:   slog.AllLevels,
		BuffMode: BuffModeLine,
		BuffSize: DefaultBufferSize,
		// rotate file settings
		MaxSize:    rotatefile.DefaultMaxSize,
		RotateTime: rotatefile.EveryHour,
		// old files clean settings
		BackupNum:  rotatefile.DefaultBackNum,
		BackupTime: rotatefile.DefaultBackTime,
	}

	return c.WithConfigFn(fns...)
}

// With more config settings func
func (c *Config) With(fns ...ConfigFn) *Config {
	return c.WithConfigFn(fns...)
}

// WithConfigFn more config settings func
func (c *Config) WithConfigFn(fns ...ConfigFn) *Config {
	for _, fn := range fns {
		fn(c)
	}
	return c
}

func (c *Config) newLevelFormattable() slog.LevelFormattable {
	if c.LevelMode == LevelModeValue {
		return slog.NewLvFormatter(c.Level)
	}
	return slog.NewLvsFormatter(c.Levels)
}

// CreateHandler quick create a handler by config
func (c *Config) CreateHandler() (*SyncCloseHandler, error) {
	output, err := c.CreateWriter()
	if err != nil {
		return nil, err
	}

	h := &SyncCloseHandler{
		Output: output,
		// with log level and formatter
		LevelFormattable: c.newLevelFormattable(),
	}

	if c.UseJSON {
		h.SetFormatter(slog.NewJSONFormatter())
	}
	return h, nil
}

// RotateWriter build rotate writer by config
func (c *Config) RotateWriter() (output SyncCloseWriter, err error) {
	if c.MaxSize == 0 && c.RotateTime == 0 {
		return nil, errorx.Raw("slog: cannot create rotate writer, MaxSize and RotateTime both is 0")
	}

	return c.CreateWriter()
}

// CreateWriter build writer by config
func (c *Config) CreateWriter() (output SyncCloseWriter, err error) {
	if c.Logfile == "" {
		return nil, errorx.Raw("slog: logfile cannot be empty for create writer")
	}
	if c.FilePerm == 0 {
		c.FilePerm = rotatefile.DefaultFilePerm
	}

	// create a rotate config.
	if c.MaxSize > 0 || c.RotateTime > 0 {
		rc := rotatefile.EmptyConfigWith()

		// has locked on logger.write()
		rc.CloseLock = true
		rc.Filepath = c.Logfile
		rc.FilePerm = c.FilePerm
		rc.DebugMode = c.DebugMode

		// copy settings
		rc.MaxSize = c.MaxSize
		rc.RotateTime = c.RotateTime
		rc.RotateMode = c.RotateMode
		rc.BackupNum = c.BackupNum
		rc.BackupTime = c.BackupTime
		rc.Compress = c.Compress

		if c.RenameFunc != nil {
			rc.RenameFunc = c.RenameFunc
		}

		// create a rotating writer
		output, err = rc.Create()
	} else {
		output, err = fsutil.OpenAppendFile(c.Logfile, c.FilePerm)
	}

	if err != nil {
		return nil, err
	}

	// wrap buffer
	if c.BuffSize > 0 {
		output = c.wrapBuffer(output)
	}
	return
}

type flushSyncCloseWriter interface {
	FlushCloseWriter
	Sync() error
}

// wrap buffer for the writer
func (c *Config) wrapBuffer(w io.Writer) (bw flushSyncCloseWriter) {
	// if c.BuffSize == 0 {
	// 	panic("slog: buff size cannot be zero on wrap buffer")
	// }

	if c.BuffMode == BuffModeLine {
		bw = bufwrite.NewLineWriterSize(w, c.BuffSize)
	} else {
		bw = bufwrite.NewBufIOWriterSize(w, c.BuffSize)
	}
	return bw
}

//
// ---------------------------------------------------------------------------
// global config func
// ---------------------------------------------------------------------------
//

// WithLogfile setting
func WithLogfile(logfile string) ConfigFn {
	return func(c *Config) { c.Logfile = logfile }
}

// WithFilePerm setting
func WithFilePerm(filePerm fs.FileMode) ConfigFn {
	return func(c *Config) { c.FilePerm = filePerm }
}

// WithLevelMode setting
func WithLevelMode(mode slog.LevelMode) ConfigFn {
	return func(c *Config) { c.LevelMode = mode }
}

// WithLogLevel setting
func WithLogLevel(level slog.Level) ConfigFn {
	return func(c *Config) { c.Level = level }
}

// WithLogLevels setting
func WithLogLevels(levels slog.Levels) ConfigFn {
	return func(c *Config) { c.Levels = levels }
}

// WithLevelNames set levels by level names.
func WithLevelNames(names []string) ConfigFn {
	levels := make([]slog.Level, 0, len(names))
	for _, name := range names {
		levels = append(levels, slog.LevelByName(name))
	}

	return func(c *Config) {
		c.Levels = levels
	}
}

// WithRotateTime setting
func WithRotateTime(rt rotatefile.RotateTime) ConfigFn {
	return func(c *Config) { c.RotateTime = rt }
}

// WithRotateMode setting
func WithRotateMode(m rotatefile.RotateMode) ConfigFn {
	return func(c *Config) { c.RotateMode = m }
}

// WithBackupNum setting
func WithBackupNum(n uint) ConfigFn {
	return func(c *Config) { c.BackupNum = n }
}

// WithBackupTime setting
func WithBackupTime(bt uint) ConfigFn {
	return func(c *Config) { c.BackupTime = bt }
}

// WithBuffMode setting
func WithBuffMode(buffMode string) ConfigFn {
	return func(c *Config) { c.BuffMode = buffMode }
}

// WithBuffSize setting
func WithBuffSize(buffSize int) ConfigFn {
	return func(c *Config) { c.BuffSize = buffSize }
}

// WithMaxSize setting
func WithMaxSize(maxSize uint64) ConfigFn {
	return func(c *Config) { c.MaxSize = maxSize }
}

// WithCompress setting
func WithCompress(compress bool) ConfigFn {
	return func(c *Config) { c.Compress = compress }
}

// WithUseJSON setting
func WithUseJSON(useJSON bool) ConfigFn {
	return func(c *Config) { c.UseJSON = useJSON }
}
