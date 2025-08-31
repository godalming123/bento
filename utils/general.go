package utils

import (
	"io"
	"iter"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

const clearBetweenCursorAndScreenEnd = "\033[0J"

func moveCursorUp(numberOfLines int) {
	print("\033[" + strconv.Itoa(numberOfLines) + "A")
}

func GetBoolDefaultYes() bool {
	print("Y/n: ")
	char := []byte{'0'}
	input := ""
	for true {
		_, err := os.Stdin.Read(char)
		if err != nil {
			Fail(err.Error())
		}
		if char[0] == '\n' {
			break
		}
		input += string(char)
	}
	switch strings.ToLower(input) {
	case "n", "no":
		return false
	case "y", "yes", "":
		return true
	default:
		println("Expected either `y`, `n`, `yes`, `no`, or ``, but got `" + input + "`")
		return GetBoolDefaultYes()
	}
}

type InterpolationError struct {
	CharecterIndex int
	MessageLines   []string
	InputString    string
}

func (e *InterpolationError) Error() string {
	return "`" + e.InputString + "`\n" + strings.Repeat(" ", e.CharecterIndex+1) + "\n^ error occured here\n" + strings.Join(e.MessageLines, "\n")
}

const accidentalInterpolationProtectionMessage = "If you do not want to use an interpolation use `$$` instead of `$`"

func InterpolateStringLiteral(stringLiteral string, getInterpolationValue func(string) (string, error)) (string, error) {
	out := ""
	for index := 0; index < len(stringLiteral); index += 1 {
		if stringLiteral[index] == '$' {
			errMsg := []string{
				"After `$`, expected either `$` or an interpolation consisting of `{`, a string, and then `}`",
				accidentalInterpolationProtectionMessage,
			}
			index += 1
			if index >= len(stringLiteral) {
				return "", &InterpolationError{
					CharecterIndex: len(stringLiteral) - 1,
					MessageLines:   errMsg,
					InputString:    stringLiteral,
				}
			}
			switch stringLiteral[index] {
			case '$':
				out += "$"
			case '{':
				interpolationIdentStart := index + 1
				for stringLiteral[index] != '}' {
					index += 1
					if index >= len(stringLiteral) {
						return "", &InterpolationError{
							CharecterIndex: len(stringLiteral) - 1,
							MessageLines: []string{
								"Unclosed interpolation chunk",
								"Expected `}` to close the interpolation",
								accidentalInterpolationProtectionMessage,
							},
							InputString: stringLiteral,
						}
					}
				}
				interpolationValue, err := getInterpolationValue(stringLiteral[interpolationIdentStart:index])
				if err != nil {
					return "", &InterpolationError{
						CharecterIndex: interpolationIdentStart,
						MessageLines:   []string{"Invalid interpolation chunk: " + err.Error()},
						InputString:    stringLiteral,
					}
				}
				out += interpolationValue
			default:
				return "", &InterpolationError{
					CharecterIndex: index,
					MessageLines:   errMsg,
					InputString:    stringLiteral,
				}
			}
		} else {
			out += string(stringLiteral[index])
		}
	}
	return out, nil
}

// Like `panic`, except this does not print "panic: ", and it does not add whitespace to every line of the message
func Fail(lines ...string) {
	for _, line := range lines {
		os.Stderr.WriteString(line + "\n")
	}
	os.Exit(1)
}

func Collect[T any](iterator iter.Seq[T]) []T {
	result := []T{}
	for elem := range iterator {
		result = append(result, elem)
	}
	return result
}

func TrimPrefix(str string, prefix string) (string, bool) {
	if strings.HasPrefix(str, prefix) {
		return str[len(prefix):], true
	}
	return str, false
}

type logSeverity uint8

// In order from less severe to more severe
const (
	infoSeverity logSeverity = iota
	nonFatalErrorSeverity
	fatalErrorSeverity
)

type log struct {
	message  string
	severity logSeverity
}

func info(message string) log {
	return log{message: message, severity: infoSeverity}
}

func nonFatalError(message string) log {
	return log{message: message, severity: nonFatalErrorSeverity}
}

func fatalError(message string) log {
	return log{message: message, severity: fatalErrorSeverity}
}

type stateWithNotifier[dataType any] struct {
	state    *dataType
	notifier chan struct{}
}

func (s *stateWithNotifier[dataType]) setState(newState dataType) {
	*s.state = newState
	if len(s.notifier) == 0 {
		s.notifier <- struct{}{}
	}
}

type progress struct {
	contentLengthInBytes int
	contentReadInBytes   int
}

type progressReader struct {
	progress
	reader        io.ReadCloser
	OnContentRead func(progress)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.contentReadInBytes += n
	pr.OnContentRead(pr.progress)
	return n, err
}

func (pr *progressReader) Close() error {
	return pr.reader.Close()
}

func ShuffleSlice[T any](slice []T) []T {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	out := make([]T, len(slice))
	for newIndex, oldIndex := range r.Perm(len(slice)) {
		out[newIndex] = slice[oldIndex]
	}
	return out
}
