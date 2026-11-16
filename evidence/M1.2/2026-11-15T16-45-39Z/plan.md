# M1.2 plan — ledger (envelope, canonical bytes, store, schemas)

Files: internal/ledger/{jcs,json,event,bodies,schema,store,verify,project,replay,gen,mutate}.go,
cmd/court subcommands (ledger append/verify/export/mutate/gen, replay), api/schema/*.schema.json,
test/checks/M1.2/*, test/sabotage/M1.2/*, docs/M1.2.md.
Checks first: C1.2.1 RFC 8785 vectors (testdata + Appendix B + es6 sha256 stream); C1.2.2 10k chain;
C1.2.3 two-process append; C1.2.4 crash at 200 injection points; C1.2.5 schema<->type bijection;
C1.2.6 MM1.5 >=2000 ev/s. First test: JCS vector test (fails: no impl).
