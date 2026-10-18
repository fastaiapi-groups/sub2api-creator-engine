export interface Group{id:number;name:string;[key:string]:any}
export interface ApiKey{id:number;name:string;key:string;group_id:number|null;status:string;[key:string]:any}
