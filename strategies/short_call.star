def initialize(ctx):
    ctx.configure(
        underlying="NIFTY",
        start_date="2026-01-01",
        end_date="2026-06-30",
    )


def on_candle(ctx):
    if ctx.dte <= 45 and ctx.time >= "09:30":
        if "entry_order_id" not in ctx.state:
            ctx.state["entry_order_id"] = ctx.place_order(
                contract=atm_option("CALL", offset=2),
                side="SELL",
                quantity=75,
            )

    if ctx.dte <= 21 and ctx.time >= "15:15":
        if "entry_order_id" in ctx.state and "exit_order_id" not in ctx.state:
            entry = ctx.order(ctx.state["entry_order_id"])
            if entry.status == "FILLED":
                ctx.state["exit_order_id"] = ctx.place_order(
                    contract=entry.contract_id,
                    side="BUY",
                    quantity=75,
                )
