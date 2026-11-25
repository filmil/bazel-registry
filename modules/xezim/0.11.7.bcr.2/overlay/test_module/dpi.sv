// Calls a DPI-C function from a shared library loaded with --dpi-lib.
module dpi_tb;
  import "DPI-C" function int dpi_add(input int a, input int b);
  initial $display("dpi_add=%0d", dpi_add(2, 40));
endmodule
