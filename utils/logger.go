package utils

// func InitLogger(debug bool) {
// 	zerolog.SetGlobalLevel(zerolog.InfoLevel)
// 	if debug {
// 		zerolog.SetGlobalLevel(zerolog.DebugLevel)
// 	}
// 	output := zerolog.ConsoleWriter{
// 		Out:        os.Stderr,
// 		TimeFormat: time.DateTime,
// 	}
// 	log.Logger = zerolog.New(output).With().Timestamp().Logger()
// 	if debug {
// 		PMDebug = true
// 	}
// }

// func GetLogger(component string) zerolog.Logger {
// 	return log.With().Str("component", component).Logger()
// }

// func SetLogOutput(w io.Writer) {
// 	output := zerolog.ConsoleWriter{
// 		Out:        w,
// 		TimeFormat: time.RFC3339,
// 	}
// 	log.Logger = zerolog.New(output).With().Timestamp().Logger()
// }
