def initialize(ctx):
    ctx.configure(
        underlying="NIFTY",
        start_date="2026-01-01",
        end_date="2026-06-30",
    )

    ctx.state["entry_done"] = False


def on_candle(ctx):
    if ctx.dte <= 45 and ctx.time >= "09:30":
        if not ctx.state["entry_done"]:
            strike = ctx.strike_offset(2)

            ctx.place_order(
                option_type="CE",
                strike=strike,
                expiry=ctx.expiry,
                side="SELL",
                quantity=75,
            )

            ctx.state["entry_done"] = True

    if ctx.dte <= 21 and ctx.time >= "15:15":
        for position in ctx.positions:
            if position.quantity < 0:
                if not ctx.has_pending_order(position.contract):
                    ctx.place_order(
                        option_type=position.option_type,
                        strike=position.strike,
                        expiry=position.expiry,
                        side="BUY",
                        quantity=abs(position.quantity),
                    )