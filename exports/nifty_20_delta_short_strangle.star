def initialize(ctx):
    ctx.configure()
    expiries = ctx.get_expiries(
        underlying="NIFTY", start="2026-01-01", end="2026-06-30",
    )

    for expiry in expiries:
        ctx.state.clear()
        ctx.state["expiry"] = expiry

        for option_type in ["CE", "PE"]:
            ctx.state[option_type] = {
                "symbol": None,
                "exit_order": None,
                "stop_count": 0,
                "reenter": False,
                "finished": False,
            }

        ctx.simulate(expiry)


def on_candle(ctx):
    manage_option(ctx, "CE")
    manage_option(ctx, "PE")


def manage_option(ctx, option_type):
    expiry = ctx.state["expiry"]
    leg = ctx.state[option_type]
    dte = expiry.get_dte(ctx.date)
    scheduled_exit = dte <= 21 and ctx.time >= "15:15"

    if leg["finished"]:
        return

    # Wait until the closing order fills before doing anything else.
    if leg["exit_order"] != None:
        if ctx.order(leg["exit_order"]).status != "FILLED":
            return
        if not leg["reenter"]:
            leg["finished"] = True
            return
        leg["symbol"] = None
        leg["exit_order"] = None

    # Select and sell a 20-delta option.
    if leg["symbol"] == None:
        if dte > 45 or ctx.time < "09:30" or scheduled_exit:
            return
        symbol = ctx.option_chain(
            underlying=expiry.underlying, expiry=expiry.date,
            option_type=option_type, filter=filter(type="delta", value=0.20),
        )
        if symbol != None:
            ctx.place_order(tradingsymbol=symbol, quantity=-75)
            leg["symbol"] = symbol
        return

    for position in ctx.get_positions(leg["symbol"]):
        if position.status != "OPEN":
            continue

        stop_hit = False
        if position.curr_price != None:
            stop_hit = position.curr_price >= position.average_price * 1.5

        if stop_hit or scheduled_exit:
            # Only the first stop permits another entry, before 21 DTE.
            leg["reenter"] = stop_hit and leg["stop_count"] == 0 and dte > 21
            if stop_hit:
                leg["stop_count"] += 1
            leg["exit_order"] = ctx.place_order(
                tradingsymbol=leg["symbol"], quantity=-position.quantity,
            )
        break
