// A DPI-C function for dpi.sv. svDpiVersion comes from xezim itself, so the
// call checks that the binary exports its DPI symbols.
#include "svdpi.h"

int dpi_add(int a, int b) {
  return svDpiVersion() != 0 ? a + b : -1;
}
