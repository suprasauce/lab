ENTRY_TIME = "09:15"
EXIT_TIME = "15:27"
STOP_LOSS = 1560


def initialize(ctx):
    ctx.configure()
    for i in range(181):
        day = date.add("2026-01-01", i)
        ctx.state.clear()
        ctx.simulate(start_date=day, end_date=day)


def on_candle(ctx):
    if ctx.time >= EXIT_TIME:
        ctx.state["exiting"] = True

    if ctx.state.get("exiting", False):
        close_positions(ctx, 0)
        return

    if ctx.time >= ENTRY_TIME and "call_entry_trade_id" not in ctx.state:
        enter_straddle(ctx)

    # Each leg follows its own assumed OHLC path; check the entry candle too.
    for phase in range(4):
        pnl = strategy_pnl(ctx, phase)
        if pnl != None and pnl <= -STOP_LOSS:
            ctx.state["exiting"] = True
            close_positions(ctx, phase)
            return


def enter_straddle(ctx):
    expiry = ctx.get_expiry("NIFTY", "WEEKLY", 0)
    if expiry == None:
        return

    call = ctx.option_chain(
        underlying="NIFTY", expiry=expiry.date, option_type="CE",
        filter=filter(type="offset", value=0),
    )
    put = ctx.option_chain(
        underlying="NIFTY", expiry=expiry.date, option_type="PE",
        filter=filter(type="offset", value=0),
    )
    if call == None or put == None:
        return

    call_candle = ctx.get_candle(call)
    put_candle = ctx.get_candle(put)
    if call_candle == None or put_candle == None:
        return

    for name, symbol, candle in [("call", call, call_candle), ("put", put, put_candle)]:
        ctx.state[name + "_symbol"] = symbol
        ctx.state[name + "_entry_trade_id"] = ctx.place_trade(
            tradingsymbol=symbol, quantity=-1, price=candle.open,
        )


def phase_price(candle, phase):
    if candle == None:
        return None
    if phase == 0:
        return candle.open
    if phase == 3:
        return candle.close

    high = candle.high
    low = candle.low
    if high == None:
        return low
    if low == None:
        return high

    # High wins ties. Pair the legs by phase, not by matching High to High.
    high_first = high - candle.open <= candle.open - low
    if phase == 1:
        return high if high_first else low
    return low if high_first else high


def strategy_pnl(ctx, phase):
    total = 0
    for name in ["call", "put"]:
        symbol = ctx.state.get(name + "_symbol")
        if symbol == None:
            continue
        for position in ctx.get_positions(symbol):
            total += position.realized_pnl
            if position.quantity != 0:
                price = phase_price(position.candle, phase)
                if price == None:
                    return None
                total += (price - position.average_price) * position.quantity * ctx.get_lot_size(symbol)
    return total


def close_positions(ctx, phase):
    for name in ["call", "put"]:
        symbol = ctx.state.get(name + "_symbol")
        if symbol == None:
            continue
        for position in ctx.get_positions(symbol):
            if position.quantity != 0:
                price = phase_price(position.candle, phase)
                if price != None:
                    ctx.state[name + "_exit_trade_id"] = ctx.place_trade(
                        tradingsymbol=symbol,
                        quantity=-position.quantity,
                        price=price,
                    )
