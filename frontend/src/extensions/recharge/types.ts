export interface Package { id: string; name: string; amount_cents: number; credit: string }
export interface Order { paid_amount_cents?: number | null; credited_credit?: string | null; id: string; package_id: string; amount_cents: number; credit: string; payment_type: string; status: string; created_at: string; paid_at: string | null; reversed_at: string | null; gateway_status: string }
export interface Log { order_id: string; event: string; detail: string; created_at: string }
export interface Checkout { order: Order; state: string; qr_code?: string; qr_kind?: 'text' | 'image' }
