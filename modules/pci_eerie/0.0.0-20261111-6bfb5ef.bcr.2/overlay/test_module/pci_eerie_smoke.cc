// Links the verilated soft core, evaluates it once and exits. Linking is
// what makes Bazel run verilator on the core, so this test is the lint
// check: verilator stops on any warning that waivers.vlt.v does not waive.
#include <memory>

#include "Vpci_eerie.h"
#include "verilated.h"

int main(int argc, char** argv) {
  auto context = std::make_unique<VerilatedContext>();
  context->commandArgs(argc, argv);
  auto top = std::make_unique<Vpci_eerie>(context.get());
  top->eval();
  top->final();
  return context->gotError() ? 1 : 0;
}
