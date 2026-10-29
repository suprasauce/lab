def initialize(ctx):
    ctx.configure(
        underlying="NIFTY",
        start_date="2026-04-01",
        end_date="2026-06-30",
    )

def on_candle(ctx):
    if ctx.dte <= 45:
        ctx.sell_call("CE", quantity=75, offset=2)
    if ctx.dte <= 21:
        ctx.close("CE")
