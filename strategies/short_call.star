def initialize(ctx):
    ctx.configure(
        underlying="NIFTY",
        start_date="2025-01-01",
        end_date="2025-12-31",
    )
    ctx.register_leg("CE", quantity=75)

def on_candle(ctx):
    if ctx.dte == 45:
        ctx.sell_call("CE", offset=2)
    if ctx.dte == 21:
        ctx.close("CE")
