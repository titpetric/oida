package oida

import "github.com/titpetric/oida/model"

// Recorder is the substitutable surface of Tracer: the write side the
// instrumentation records through, and the read side the debug front end
// renders from. Code that only needs to record and read back traces can depend
// on this interface instead of the concrete tracer.
type Recorder = model.Recorder
