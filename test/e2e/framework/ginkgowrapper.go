package framework

type label struct {
	// parts get concatenated with ":" to build the full label.
	parts []string
	// extra is an optional fully-formed extra label.
	extra string
	// explanation gets set for each label to help developers
	// who pass a label to a ginkgo function. They need to use
	// the corresponding framework function instead.
	explanation string
}

func newLabel(parts ...string) label {
	return label{
		parts:       parts,
		explanation: "If you see this as part of an 'Unknown Decorator' error from Ginkgo, then you need to replace the ginkgo.It/Context/Describe call with the corresponding framework.It/Context/Describe or (if available) f.It/Context/Describe.",
	}
}

// WithDisruptive is a shorthand for the corresponding package function.
func (f *Framework) WithDisruptive() interface{} {
	return withDisruptive()
}

func withDisruptive() interface{} {
	return newLabel("Disruptive")
}

// WithSerial is a shorthand for the corresponding package function.
func (f *Framework) WithSerial() interface{} {
	return withSerial()
}

func withSerial() interface{} {
	return newLabel("Serial")
}
