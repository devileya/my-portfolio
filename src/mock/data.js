import { nanoid } from 'nanoid';

// HEAD DATA
export const headData = {
  title: 'Arif Fadly Siregar', // e.g: 'Name | Developer'
  lang: 'en', // e.g: en, id
  description: '', // e.g: Welcome to my website
};

// HERO DATA
export const heroData = {
  title: '',
  name: 'Arif Fadly Siregar',
  subtitle: `Software Engineer and Tech Geek`,
  cta: '',
};

// ABOUT DATA
export const aboutData = {
  img: 'photo.png',
  paragraphOne: `I am Software Engineer that enthusiast about Mobile Development, Web Development (front-end and back-end), Data Science/Analytics and Scripting`,
  paragraphTwo: `I am passionate about building scalable software, creating effective solutions, and learning every day to grow professionally in the IT field.`,
  paragraphThree: `Click button 'resume' below to see my resume or scroll down to see my projects`,
  resume: 'https://drive.google.com/file/d/12QaCGziCVv0IDAbqxGWN8Ff2Ja2_N_7J/view?usp=sharing', // if no resume, the button will not show up
};

// PROJECTS DATA
export const projectsData = [
  {
    id: nanoid(),
    img: 'appen-mobile.jpg',
    title: 'Appen (Android)',
    info: `I support and maintenance the app and also create audio and video collection prompt for this app`,
    info2: `Technology: Android, iOS, React Native, Axios, Code push, Azure, NPM`,
    url: 'https://play.google.com/store/apps/details?id=id.dana',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'geolancer-mobile.jpg',
    title: 'Geolancer (Android)',
    info: `I develop many features in Geolancer app such as Address Standardization, Image Blurry Detection, MBP, Audio Video Collection and I also check the security of apps by reverse-engineering on that using JADX and GHIDRA`,
    info2: `Technology: Android, Kotlin, MVVM, HILT, Play Integrity, Mixpanel, Twilio, Firebase, JADX, GHIDRA`,
    url: 'https://play.google.com/store/apps/details?id=id.dana',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'dana-mobile.jpg',
    title: 'DANA (Android)',
    info: `I develop many features in DANA app such as DANA Viz (able to pay using face authentication), Contact Injection, Wallet, support and maintenance emoney integration using NFC`,
    info2: `Technology: Android, Kotlin, Java, MVP, Firebase, Split.io, Branch.io, Alipay, DexGuard, Modularization, Dagger, Epoxy`,
    url: 'https://play.google.com/store/apps/details?id=id.dana',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'peta-mobile.png',
    title: 'PETA REG-SUMKAL (Android, iOS, Web)',
    info: `I develop PETA REG-SUMKAL with my own team using kotlin for android and swift for ios. This app for maintain and give report about PLN assets in Sumatera-Kalimantan Regional.
    Features in this app includes authentication, track current location, analytics (Charts), notification and many more`,
    info2: `Technology: Android, iOS, Kotlin, Swift, MVVM, Koin, Coroutine, Retrofit, Alamofire, Firebase, Google Maps, Charts`,
    url: 'https://play.google.com/store/apps/details?id=com.pln.peta',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'pdkb-mobile.jpg',
    title: 'PDKB SUMATERA (Android, iOS, Web)',
    info: `I develop PDKB SUMATERA with my own team using flutter. This is inventory app that handle incoming/outcoming item with scan QR-CODE for internal PLN Sumatera`,
    info2: `Technology: Hybrid (Android, iOS), Flutter, Dart, BLOC, QR-Code Scanner, Firebase`,
    url: 'https://play.google.com/store/apps/details?id=com.pln.pdkb',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'sehatq-android.png',
    title: 'SehatQ App (Android)',
    info: `I develop sehatQ android app with sehatQ android team, sehatQ app is health application that has many feature like 
    telemedicine (chat and video call), buy medicine, book a doctor, check up routine, health record and many more`,
    info2: `Technology: Android, Kotlin, MVVM, Koin, Coroutine, Retrofit, Firebase, Google Maps, Sendbird`,
    url: 'https://play.google.com/store/apps/details?id=com.she.sehatq',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'doctorapp-android.png',
    title: 'Doctor App by SehatQ (Android)',
    info: `I develop doctor app with sehatQ android team, this app for doctor to interact with customer or patient sehatQ
    with our telemedicine feature, so doctor can chat or video call with patient and also give diagnose and drug recommendation`,
    info2: 'Technology: Android, Kotlin, MVVM, Dagger, Coroutine, Retrofit, Firebase, Sendbird',
    url: 'https://play.google.com/store/apps/details?id=com.she.sehatqdoctor.android',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'nusa-android.png',
    title: 'Nusa (Android)',
    info: `I developed this app with my team and this is project from germany, this app about to book vacation activities 
    like surfing, snorkling, scuba diving and this already done but cannot be publish because COVID-19 pandemic`,
    info2: `Technology: Android, Java, MVVM, Dagger, RxJava2, Lombok, Firebase, Google Maps`,
    url: '',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'biznetwfm-android.png',
    title: 'Biznet Work Force Management (Android)',
    info: `I developed this app with Biznet mobile app team, this app make the engineer of biznet work more easier and controlable. 
    Because there many feature like get ticket directly, inventory management, tracking engineer, register modem automatically, 
    internet speed test, digital signature and many more`,
    info2: `Technology: Android, Kotlin, MVP, MVVM, Room, Koin, RxJava2, Coroutine, Retrofit, Firebase, Google Maps`,
    url: '',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'biznetintranet-flutter.png',
    title: 'Biznet Intranet (Android and IOS)',
    info: `I developed this app with Biznet mobile app team, this app for biznet employee only`,
    info2: `Technology: Flutter, Dart, Bloc, Dio`,
    url: '',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'mybiznet-app.PNG',
    title: 'MyBiznet (Android and IOS)',
    info: `I developed this app with some Biznet vendor, this app for Biznet's customer`,
    info2:
      'Technology: Android, Kotlin, MVP, Dagger, RXJava, Retrofit, Firebase, Google Maps, IOS, Swift',
    url: 'https://play.google.com/store/apps/details?id=biznet.id.android',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'gabut-android.png',
    title: 'Gabut (Android)',
    info: `This app is communication app like whatsApp or telegram that has many feature like timeline and chat`,
    info2: `Technology: Android, Java, MVC, Dagger, RXJava, Retrofit, Firebase, PHP, Yii`,
    url: 'https://play.google.com/store/apps/details?id=lagi.gabut.inc',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'cv-web.png',
    title: 'Simple Portfolio (Web)',
    info: `I developed this web for my profile and portfolio only`,
    info2: `Technology: React, Gatsby, Javascript, HTML, CSS`,
    url: '/',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'cabana-web.png',
    title: 'Cabana Surf and Stay (Web)',
    info: `This web is about cabana that has many feature like reserve hotel, blog, weather and swell information and show events in cabana`,
    info2: `Technology: CodeIgniter, PHP, MySQL, JQuery, HTML, CSS`,
    url: 'https://cabanasurfandstay.com',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'danalaut-web.png',
    title: 'Danalaut (Web)',
    info: `I developed this web with Seva's team, this web is about peer-to-peer lending for Indonesia Maritim Economic`,
    info2: `Technology: Angular 4, Typescript, Symfony, PHP, PostgreSQL, JWT Auth, REST API, HTML, CSS`,
    url: 'https://danalaut.id/',
    repo: '', // if no repo, the button will not show up
  },
  {
    id: nanoid(),
    img: 'virallo-web.png',
    title: 'Virallo (Web)',
    info: `I developed this web with Seva's team, this web is about brand campaign and endorse management`,
    info2: `Technology: Angular 4, Typescript, Laravel, PHP, PostgreSQL, JWT Auth, REST API, HTML, CSS`,
    url: 'https://virallo.id/',
    repo: '', // if no repo, the button will not show up
  },
];

// CONTACT DATA
export const contactData = {
  cta: 'Feel free to contact me via email',
  btn: 'Email Me',
  email: 'fadlyarif77@gmail.com',
};

// FOOTER DATA
export const footerData = {
  networks: [
    {
      id: nanoid(),
      name: 'linkedin',
      url: 'https://www.linkedin.com/in/arif-fadly-siregar-610279137/',
    },
    {
      id: nanoid(),
      name: 'github',
      url: 'https://github.com/devileya/',
    },
  ],
};

// Github start/fork buttons
export const githubButtons = {
  isEnabled: true, // set to false to disable the GitHub stars/fork buttons
};
