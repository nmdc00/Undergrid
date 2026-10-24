import type {ObjectValue} from "./api";
export type PartialReview={intentionID:string;leadID:string;revision:number;version:number;etag:string;sourceVersion:string;projectionHash:string;reportHash:string;requiredFields:string[]};
export function PartialSatisfaction(_: {review:PartialReview;csrf:string;onRecorded:(result:ObjectValue)=>void;onUnauthorized:()=>void;onUnavailable:()=>void}){
 return <section aria-label="Record partial fulfillment"><button>Save unmet remainder</button></section>;
}
