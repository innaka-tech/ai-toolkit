package checkrun

import "io"

func ioMulti(a, b io.Writer) io.Writer { return io.MultiWriter(a, b) }
