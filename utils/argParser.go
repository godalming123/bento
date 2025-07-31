package utils

import "os"

type Argument struct {
	Desc  string
	Value *string
}

func TakeOneArg(index *int, argDescription string) string {
	if *index >= len(os.Args) {
		Fail("Expected another argument: " + argDescription)
	}
	out := os.Args[*index]
	*index += 1
	return out
}

func TakeArgs(index *int, args []Argument) {
	lenOfArgsLeft := len(os.Args) - *index
	if len(args) > lenOfArgsLeft {
		err := make([]string, 0, len(args)+2)
		err = append(err, "Expected the following additional arguments:")
		for _, arg := range args[lenOfArgsLeft:] {
			err = append(err, " - "+arg.Desc)
		}
		if lenOfArgsLeft > 0 {
			err = append(err, "After the following already specified arguments:")
			for i, arg := range args[:lenOfArgsLeft] {
				err = append(err, " - "+arg.Desc+": "+os.Args[*index+i])
			}
		}
		Fail(err...)
	}
	for _, arg := range args {
		if arg.Value != nil {
			*arg.Value = os.Args[*index]
		}
		*index += 1
	}
}

func ExpectAllArgsParsed(index int) {
	if index < len(os.Args) {
		Fail("Did not expect any extra arguments after `" + os.Args[index-1] + "`")
	}
}
