CREATE TABLE households (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_name TEXT NOT NULL,
    address TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE waste_pickups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id UUID NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
    type TEXT NOT NULL CHECK (type IN ('organic', 'plastic', 'paper', 'electronic')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'scheduled', 'completed', 'canceled')),
    pickup_date TIMESTAMPTZ NULL,
    safety_check BOOLEAN NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id UUID NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
    waste_id UUID NOT NULL REFERENCES waste_pickups(id) ON DELETE RESTRICT,
    amount NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    payment_date TIMESTAMPTZ NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'failed')),
    proof_file_url TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT payments_waste_id_unique UNIQUE (waste_id)
);

CREATE INDEX waste_pickups_household_status_idx ON waste_pickups (household_id, status);
CREATE INDEX payments_household_status_idx ON payments (household_id, status);
CREATE INDEX payments_payment_date_idx ON payments (payment_date);
