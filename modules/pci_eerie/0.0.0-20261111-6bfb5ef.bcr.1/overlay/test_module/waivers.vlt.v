// Lint waivers for pci_eerie 6bfb5ef, at the default warning level.
// Each warning was read against its source line, and none is an
// upstream bug. The width waivers cover whole files: this edition pins
// one upstream commit, and a new commit gets a new edition and a new
// review of its warnings. Upstream targets Vivado and xsim, which follow the
// Verilog-2005 sizing rules without these warnings.
//
// This is a .v file because rules_verilator passes only .v and .sv
// files on. The `verilator_config line switches to the configuration
// language. No comment line here may start with that tool's name: it
// reads such a comment as a pragma.
`verilator_config

// Zero-extensions of narrower operands: buffer addresses widened to the
// 13-bit port of the hard block, credit counts added to wider counters,
// and capability pointers widened to 10 bits.
lint_off -rule WIDTHEXPAND -file "*/rtl/eerie_cfg.v"
lint_off -rule WIDTHEXPAND -file "*/rtl/eerie_dll.v"
lint_off -rule WIDTHEXPAND -file "*/rtl/eerie_tl_rx.v"
lint_off -rule WIDTHEXPAND -file "*/rtl/pci_eerie.v"

// Lane and ring indices computed in 32 bits or masked with & 15 and & 7
// before use, so the extra high bits are always zero.
lint_off -rule WIDTHEXPAND -file "*/rtl/eerie_pl_deskew.v"
lint_off -rule WIDTHEXPAND -file "*/rtl/eerie_pl_rx_ml.v"
lint_off -rule WIDTHTRUNC -file "*/rtl/eerie_pl_rx_ml.v"
lint_off -rule WIDTHTRUNC -file "*/rtl/eerie_pl_tx_ml.v"
lint_off -rule WIDTHEXPAND -file "*/rtl/eerie_ltssm.v"
lint_off -rule WIDTHTRUNC -file "*/rtl/eerie_ltssm.v"

// Truncations whose dropped bits are zero: parameters assigned to their
// field width, the 64-bit X-to-0 helper xz0() assigned to each port's
// width, and credit sums of at most 256, which fit in 10 bits.
lint_off -rule WIDTHTRUNC -file "*/rtl/eerie_cfg.v"
lint_off -rule WIDTHTRUNC -file "*/rtl/eerie_dll.v"
lint_off -rule WIDTHTRUNC -file "*/rtl/eerie_tl_rx.v"
lint_off -rule WIDTHTRUNC -file "*/rtl/pci_eerie.v"

// u_v, u_d, u_k and u_e at eerie_pl_rx_ml.v:46 are scratch variables. They
// are written and then read only in the x2 branch of the same always
// block, so they hold no state from one evaluation to the next.
lint_off -rule LATCH -file "*/rtl/eerie_pl_rx_ml.v" -match "*.u_v'*"
lint_off -rule LATCH -file "*/rtl/eerie_pl_rx_ml.v" -match "*.u_d'*"
lint_off -rule LATCH -file "*/rtl/eerie_pl_rx_ml.v" -match "*.u_k'*"
lint_off -rule LATCH -file "*/rtl/eerie_pl_rx_ml.v" -match "*.u_e'*"

// The testbench copies each lane's 8-bit skew from SKEW into an integer.
lint_off -rule WIDTHEXPAND -file "*/tb/unit/ml_phy_tb.v"
