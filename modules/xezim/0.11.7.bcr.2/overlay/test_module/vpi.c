// A VPI module. vpi_printf is C-variadic and lives in xezim's C shim, so this
// checks that the shim is linked in and exported.
#include "vpi_user.h"

static void hello(void) { vpi_printf("vpi_hello=%d\n", 42); }

void (*vlog_startup_routines[])(void) = {hello, 0};
