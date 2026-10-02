import { Link } from 'react-router-dom'
export default function SectionTitle({eyebrow,title,description,to}){return <div className="section-head"><div><div className="eyebrow">{eyebrow}</div><h2>{title}</h2>{description&&<p>{description}</p>}</div>{to&&<Link to={to} className="text-link">View all →</Link>}</div>}
