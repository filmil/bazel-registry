// Stops with $fatal, so the simulator must exit nonzero.
module fatal_tb;
  initial begin
    #1;
    $fatal(1, "expected failure");
  end
endmodule
