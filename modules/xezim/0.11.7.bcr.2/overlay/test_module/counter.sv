// Counts to 10 on a clock, then finishes.
module counter_tb;
  logic clk = 0;
  logic [3:0] count = 0;

  always #5 clk = ~clk;

  always @(posedge clk) begin
    count <= count + 1;
    if (count == 4'd9) begin
      $display("count=%0d", count + 1);
      $finish;
    end
  end
endmodule
