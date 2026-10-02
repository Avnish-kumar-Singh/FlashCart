import { Outlet } from 'react-router-dom'
import Header from './Header'
import Footer from './Footer'
import AnnouncementBanner from './AnnouncementBanner'
import FestivalBanner from './FestivalBanner'

export default function Layout() {
  return (
    <>
      <FestivalBanner />
      <AnnouncementBanner />
      <Header />
      <main>
        <Outlet />
      </main>
      <Footer />
    </>
  )
}
