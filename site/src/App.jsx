import { Routes, Route } from 'react-router-dom'
import Hero from './components/Hero'
import Problem from './components/Problem'
import Solution from './components/Solution'
import Features from './components/Features'
import PlatformTeams from './components/PlatformTeams'
import HowItWorks from './components/HowItWorks'
import WhyNow from './components/WhyNow'
import CTA from './components/CTA'
import Footer from './components/Footer'
import DocsLayout from './docs/DocsLayout'

function Home() {
  return (
    <div className="app">
      <Hero />
      <Problem />
      <Solution />
      <Features />
      <PlatformTeams />
      <HowItWorks />
      <WhyNow />
      <CTA />
      <Footer />
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Home />} />
      <Route path="/docs/*" element={<DocsLayout />} />
    </Routes>
  )
}
