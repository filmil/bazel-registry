// verilator --main generates main() for the gate-level testbench.
// Verilator's runtime still asks for sc_time_stamp(), so this file
// defines it.
#include <verilated.h>

double sc_time_stamp() { return 0; }
