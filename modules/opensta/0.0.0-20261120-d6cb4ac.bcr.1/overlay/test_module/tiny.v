// Two flip-flops with two buffers between them.
module top (clk, d, q);
  input clk, d;
  output q;
  wire q1, b1, b2;
  DFF r1 (.CLK(clk), .D(d), .Q(q1));
  BUF u1 (.A(q1), .Y(b1));
  BUF u2 (.A(b1), .Y(b2));
  DFF r2 (.CLK(clk), .D(b2), .Q(q));
endmodule
