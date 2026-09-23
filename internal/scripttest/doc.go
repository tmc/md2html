// Package scripttest runs rsc.io/script tests against commands that
// listen on TCP ports.
//
// [Test] rewrites the port numbers written in each script to free ports
// reserved at run time, so the scripts can run in parallel, and then runs
// them. [BackgroundCmd] starts a program in the background and stops it
// with SIGTERM rather than SIGKILL when the script ends, so a server can
// shut down cleanly and write its coverage data.
package scripttest
