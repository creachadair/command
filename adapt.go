package command

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	envType         = reflect.TypeOf((*Env)(nil))
	errType         = reflect.TypeOf((*error)(nil)).Elem()
	stringType      = reflect.TypeOf(string(""))
	stringSliceType = reflect.TypeOf([]string(nil))
)

// Adapt adapts a more general function to the type signature of a Run
// function. The value of fn must be a function with a type signature like:
//
//	func(*command.Env) error
//	func(*command.Env, s1, s2 string) error
//	func(*command.Env, s1, s2 string, more ...string) error
//	func(*command.Env, s1, s2 string, rest []string) error
//
// That is, its first argument must be a *command.Env, it must return an error,
// and the rest of its arguments must be strings except the last, which may be
// a slice of strings (a "rest parameter").
//
// The adapted function checks that the arguments presented match the number of
// strings accepted by fn. If fn is variadic or has a rest parameter, at least
// as many arguments must be provided as the number of fixed parameters.
// Otherwise, the number of arguments must match exactly. If this fails, the
// adapted function reports an error without calling fn.  Otherwise, the
// adapter calls fn and returns its result.
//
// Adapt will panic if fn is not a function of a supported type.
func Adapt(fn any) func(*Env) error {
	r, err := checkAdapt(fn)
	if err != nil {
		panic(fmt.Sprintf("invalid argument: %v", err))
	}
	return r
}

func checkAdapt(fn any) (func(*Env) error, error) {
	// Case 1: The function accepts no arguments.
	if fz, ok := fn.(func(*Env) error); ok {
		return func(env *Env) error {
			if len(env.Args) != 0 {
				return env.Usagef("extra arguments after command: %q", env.Args)
			}
			return fz(env)
		}, nil
	}

	// Require that fn has the form func(*Env, ...) error.
	t := reflect.TypeOf(fn)
	if t.Kind() != reflect.Func {
		return nil, errors.New("not a function")
	}
	ni := t.NumIn()
	if ni == 0 || t.In(0) != envType {
		return nil, fmt.Errorf("first argument must be %v", envType)
	} else if t.NumOut() != 1 || t.Out(0) != errType {
		return nil, fmt.Errorf("return type must be %v", errType)
	}

	// Require that the arguments be strings, save that the last argument may be
	// a slice of strings.
	var hasRest bool
	for i := 1; i < ni; i++ {
		ti := t.In(i)
		if ti == stringType {
			continue
		} else if i+1 == ni && ti == stringSliceType {
			hasRest = true
			continue
		}
		return nil, fmt.Errorf("argument %d is type %v, not string", i+1, ti)
	}

	fv := reflect.ValueOf(fn)
	argc := ni - 1

	call := fv.Call
	if t.IsVariadic() {
		call = fv.CallSlice
	}

	// Case 2: A variadic function, or one with a rest slice.
	if hasRest {
		return func(env *Env) error {
			if len(env.Args) < argc-1 {
				return env.Usagef("wrong number of arguments: got %d, want at least %d", len(env.Args), argc-1)
			}
			args := append(packValues(env, argc-1), reflect.ValueOf(env.Args[argc-1:]))
			return unpackError(call(args))
		}, nil
	}

	// Case 3: A fixed-positional function.
	return func(env *Env) error {
		if len(env.Args) != argc {
			return env.Usagef("wrong number of arguments: got %d, want %d", len(env.Args), argc)
		}
		args := packValues(env, argc)
		return unpackError(call(args))
	}, nil
}

func packValues(env *Env, n int) []reflect.Value {
	vals := make([]reflect.Value, n+1)
	vals[0] = reflect.ValueOf(env)
	for i, arg := range env.Args[:n] {
		vals[i+1] = reflect.ValueOf(arg)
	}
	return vals
}

func unpackError(outs []reflect.Value) error {
	if v := outs[0].Interface(); v != nil {
		return v.(error)
	}
	return nil
}
